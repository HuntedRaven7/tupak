// Package flathub talks to the Flathub v2 API with a local disk cache.
//
// API notes (verified against the live v2 API):
//   - GET  /api/v2/appstream          -> JSON array of app ID strings (not objects)
//   - GET  /api/v2/appstream/{id}     -> full object with `categories` []string,
//     `releases` [{version,...}], nested `urls` object
//   - POST /api/v2/search             -> {hits, totalHits, facetDistribution...};
//     hits carry `main_categories` as a single lowercase string plus
//     `sub_categories` []string. `facetDistribution.main_categories` gives
//     per-category counts in one request.
//   - GET  /api/v2/collection/category/{Slug}?page=N -> {hits, totalPages,...},
//     250 hits per page, same hit shape as search.
package flathub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// APIBase is the Flathub v2 REST endpoint.
	APIBase = "https://flathub.org/api/v2"
	// cacheTTL controls how long the catalog is reused.
	cacheTTL = 24 * time.Hour
	// catalogCache is versioned so old shapes never poison new reads.
	catalogCache = "catalog_v2.json"
)

var httpClient = &http.Client{Timeout: 30 * time.Second }

// App is the subset of Flathub metadata tupak needs (no screenshots).
// JSON tags match the normalized cache form; live API shapes are handled
// by appWire + UnmarshalJSON below.
type App struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Developer   string   `json:"developer_name"`
	License     string   `json:"project_license"`
	Version     string   `json:"version"`
	Categories  []string `json:"categories"`
	Keywords    []string `json:"keywords"`
	Homepage    string   `json:"homepage"`
	Bugtracker  string   `json:"bugtracker"`
	Donation    string   `json:"donation"`
	Icon        string   `json:"icon"`
	UpdatedAt   int64    `json:"updated_at"`
	Installs    int      `json:"installs_last_month"`
}

// flexCats decodes `main_categories`, which the API returns as a single
// lowercase string ("game") in search/collection hits, an array in some
// payloads, or null.
type flexCats []string

// UnmarshalJSON accepts a string, an array of strings, or null.
func (f *flexCats) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*f = nil
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var one string
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		if one == "" {
			*f = nil
			return nil
		}
		*f = flexCats{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*f = flexCats(many)
	return nil
}

// appWire mirrors every JSON shape we consume; App is built from it so the
// two category keys (`categories` vs `main_categories`), nested `urls` and
// `releases[0].version` all land in one place.
type appWire struct {
	ID          string            `json:"id"`
	AppID       string            `json:"app_id"`
	Name        string            `json:"name"`
	Summary     string            `json:"summary"`
	Description string            `json:"description"`
	Developer   string            `json:"developer_name"`
	License     string            `json:"project_license"`
	Version     string            `json:"version"`
	Cats        flexCats          `json:"categories"`
	MainCats    flexCats          `json:"main_categories"`
	SubCats     []string          `json:"sub_categories"`
	Keywords    []string          `json:"keywords"`
	Icon        string            `json:"icon"`
	UpdatedAt   int64             `json:"updated_at"`
	Installs    int               `json:"installs_last_month"`
	URLs        map[string]string `json:"urls"`
	Type        string            `json:"type"`
	Releases    []struct {
		Version string `json:"version"`
	} `json:"releases"`
}

// toApp normalizes a wire record into an App.
func (w appWire) toApp() App {
	a := App{
		ID:          w.ID,
		Name:        w.Name,
		Summary:     w.Summary,
		Description: w.Description,
		Developer:   w.Developer,
		License:     w.License,
		Version:     w.Version,
		Keywords:    w.Keywords,
		Icon:        w.Icon,
		UpdatedAt:   w.UpdatedAt,
		Installs:    w.Installs,
	}
	if a.ID == "" {
		a.ID = w.AppID
	} else if w.AppID != "" && strings.Contains(w.AppID, ".") {
		// Search/collection hits carry `id` underscore-joined
		// ("org_gnome_gedit") alongside the real dotted `app_id`.
		a.ID = w.AppID
	}
	// Detail objects use `categories`; hits use `main_categories` (+ subs).
	cats := []string(w.Cats)
	if len(cats) == 0 {
		cats = append(cats, []string(w.MainCats)...)
		cats = append(cats, w.SubCats...)
	}
	a.Categories = cats
	if a.Version == "" && len(w.Releases) > 0 {
		a.Version = w.Releases[0].Version
	}
	if w.URLs != nil {
		a.Homepage = w.URLs["homepage"]
		a.Bugtracker = w.URLs["bugtracker"]
		a.Donation = w.URLs["donation"]
	}
	return a
}

// UnmarshalJSON decodes any Flathub app object shape into an App.
func (a *App) UnmarshalJSON(data []byte) error {
	var w appWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*a = w.toApp()
	return nil
}

// MarshalJSON encodes the normalized form (used for the disk cache).
func (a App) MarshalJSON() ([]byte, error) {
	type plain App
	return json.Marshal(plain(a))
}

// decodeApp decodes one catalog entry (live shape or cache shape — the
// wire decoder handles both since the cache uses the same key names).
func decodeApp(data []byte) (App, error) {
	var w appWire
	if err := json.Unmarshal(data, &w); err != nil {
		return App{}, err
	}
	return w.toApp(), nil
}

// DisplayName falls back to the app ID when the name is empty.
func (a App) DisplayName() string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}

// cachePath returns ~/.cache/tupak/<name>.
func cachePath(name string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "tupak")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func readCatalog(ttl time.Duration) ([]App, bool) {
	p, err := cachePath(catalogCache)
	if err != nil {
		return nil, false
	}
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > ttl {
		return nil, false
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	var raw []json.RawMessage
	if err := json.NewDecoder(f).Decode(&raw); err != nil || len(raw) == 0 {
		return nil, false
	}
	apps := make([]App, 0, len(raw))
	for _, r := range raw {
		a, err := decodeApp(r)
		if err != nil || a.ID == "" {
			continue
		}
		apps = append(apps, a)
	}
	if len(apps) == 0 {
		return nil, false
	}
	return apps, true
}

func writeCatalog(apps []App) {
	p, err := cachePath(catalogCache)
	if err != nil {
		return
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(apps); err != nil {
		return
	}
	_ = os.WriteFile(p, buf.Bytes(), 0o644)
}

// postSearch runs a POST /api/v2/search and returns the raw envelope.
func postSearch(ctx context.Context, payload map[string]any) (searchEnvelope, error) {
	var env searchEnvelope
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, APIBase+"/search", bytes.NewReader(body))
	if err != nil {
		return env, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return env, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return env, fmt.Errorf("flathub search: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return env, err
	}
	return env, nil
}

type searchEnvelope struct {
	Hits []appWire `json:"hits"`
	Facets struct {
		MainCats map[string]int `json:"main_categories"`
	} `json:"facetDistribution"`
}

// CategoryCounts returns per-category app counts with a single request.
func CategoryCounts(ctx context.Context) (map[string]int, error) {
	env, err := postSearch(ctx, map[string]any{"query": "", "page": 1})
	if err != nil {
		return nil, err
	}
	return env.Facets.MainCats, nil
}

type collectionEnvelope struct {
	Hits       []appWire `json:"hits"`
	TotalPages int       `json:"totalPages"`
}

// fetchCategoryPage pulls one page of a collection category.
// Both page and per_page are required together — either alone is a 400.
func fetchCategoryPage(ctx context.Context, slug string, page int) (collectionEnvelope, error) {
	var env collectionEnvelope
	u := APIBase + "/collection/category/" + url.PathEscape(slug) +
		"?page=" + fmt.Sprint(page) + "&per_page=250"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return env, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return env, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return env, fmt.Errorf("flathub category %s: %s", slug, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return env, err
	}
	return env, nil
}

// FetchCategory returns every app in one main category (paginated, 250/page).
func FetchCategory(ctx context.Context, slug string) ([]App, error) {
	first, err := fetchCategoryPage(ctx, slug, 1)
	if err != nil {
		return nil, err
	}
	wires := first.Hits
	for p := 2; p <= first.TotalPages; p++ {
		env, err := fetchCategoryPage(ctx, slug, p)
		if err != nil {
			return nil, err
		}
		wires = append(wires, env.Hits...)
	}
	apps := make([]App, 0, len(wires))
	for _, w := range wires {
		a := w.toApp()
		if a.ID == "" {
			continue
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// FetchAll returns the union of all main categories, cached for 24h.
// ~15 requests on a cold cache (1 facet + ~2 pages x ~10 categories),
// fetched concurrently with a bounded worker pool.
func FetchAll(ctx context.Context) ([]App, error) {
	if apps, ok := readCatalog(cacheTTL); ok {
		return apps, nil
	}
	counts, err := CategoryCounts(ctx)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(counts))
	for slug := range counts {
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		for _, c := range Categories {
			if c.Slug != "All" {
				slugs = append(slugs, c.Slug)
			}
		}
	}

	const workers = 4
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var wg sync.WaitGroup
	seen := map[string]bool{}
	var apps []App
	var firstErr error

	for _, slug := range slugs {
		wg.Add(1)
		go func(slug string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			got, err := FetchCategory(ctx, slug)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			for _, a := range got {
				if seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				apps = append(apps, a)
			}
		}(slug)
	}
	wg.Wait()
	if len(apps) == 0 && firstErr != nil {
		return nil, firstErr
	}
	writeCatalog(apps)
	return apps, nil
}

// Get fetches full metadata for one app ID.
func Get(ctx context.Context, appID string) (App, error) {
	var a App
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/appstream/"+url.PathEscape(appID), nil)
	if err != nil {
		return a, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return a, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return a, fmt.Errorf("flathub app %s: %s: %s", appID, resp.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return a, err
	}
	if a.ID == "" {
		a.ID = appID
	}
	return a, nil
}

// SearchType filters search results.
type SearchType string

const (
	SearchApps     SearchType = "apps"     // desktop apps only (default UX)
	SearchRuntimes SearchType = "runtimes" // runtime refs
	SearchBoth     SearchType = "both"     // everything
)

// Hit is one search result, normalized for the TUI.
type Hit struct {
	AppID      string
	Name       string
	Summary    string
	Developer  string
	Categories []string
	IsRuntime  bool
}

// Search queries POST /api/v2/search. Filters runtimes client-side so the
// TUI toggle (Apps / Runtimes / Both) works even if the API ignores it.
func Search(ctx context.Context, query string, typ SearchType, limit int) ([]Hit, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	env, err := postSearch(ctx, map[string]any{
		"query": query,
		"page":  1,
	})
	if err != nil {
		return nil, err
	}
	var out []Hit
	for _, w := range env.Hits {
		a := w.toApp()
		if a.ID == "" {
			continue
		}
		isRuntime := strings.Contains(strings.ToLower(w.Type), "runtime")
		switch typ {
		case SearchApps:
			if isRuntime {
				continue
			}
		case SearchRuntimes:
			if !isRuntime {
				continue
			}
		}
		out = append(out, Hit{
			AppID:      a.ID,
			Name:       a.Name,
			Summary:    a.Summary,
			Developer:  a.Developer,
			Categories: a.Categories,
			IsRuntime:  isRuntime,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// ByCategory filters a catalog by main category (case-insensitive).
// Pass "All" to return everything.
func ByCategory(apps []App, category string) []App {
	if strings.EqualFold(category, "All") || category == "" {
		return apps
	}
	var out []App
	for _, a := range apps {
		for _, c := range a.Categories {
			if strings.EqualFold(c, category) {
				out = append(out, a)
				break
			}
		}
	}
	return out
}
