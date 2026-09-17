// Package flatpak wraps the flatpak CLI so the TUI stays thin and testable.
package flatpak

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Scope selects user vs system scope. Empty means "let flatpak decide".
type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
	ScopeAuto   Scope = ""
)

// FlathubRepo is the canonical Flathub .flatpakrepo location.
const FlathubRepo = "https://dl.flathub.org/repo/flathub.flatpakrepo"

// Remote is one configured flatpak remote.
type Remote struct {
	Name    string
	URL     string
	Title   string
	Options string // raw options column, e.g. "system", "user", "disabled"
}

// IsSystem reports true when the remote is a system-wide remote.
func (r Remote) IsSystem() bool {
	return strings.Contains(r.Options, "system")
}

// InstalledApp is one installed app (apps-only by default).
type InstalledApp struct {
	ID      string // e.g. org.gnome.gedit
	Name    string
	Origin  string // remote name
	Version string
	Branch  string
}

// SearchResult is a `flatpak search` hit (used as offline fallback).
type SearchResult struct {
	Name        string
	AppID       string
	Version     string
	Branch      string
	Remotes     string
	Description string
}

func run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "flatpak", args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(errBuf.String())
		if stderr != "" {
			return "", fmt.Errorf("flatpak %s: %v: %s", strings.Join(args, " "), err, stderr)
		}
		return "", fmt.Errorf("flatpak %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

func scopeArgs(s Scope) []string {
	switch s {
	case ScopeUser:
		return []string{"--user"}
	case ScopeSystem:
		return []string{"--system"}
	default:
		return nil
	}
}

// ListRemotes returns all configured remotes in priority order.
func ListRemotes(ctx context.Context) ([]Remote, error) {
	out, err := run(ctx, "remotes", "--columns=name,url,title,options")
	if err != nil {
		return nil, err
	}
	var remotes []Remote
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		// name, url, title, options — be lenient about missing cols.
		r := Remote{}
		if len(parts) > 0 {
			r.Name = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			r.URL = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			r.Title = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			r.Options = strings.TrimSpace(parts[3])
		}
		if r.Name == "" {
			continue
		}
		remotes = append(remotes, r)
	}
	return remotes, sc.Err()
}

// HasFlathub reports whether flathub (any case) is configured.
func HasFlathub(remotes []Remote) bool {
	for _, r := range remotes {
		if strings.EqualFold(r.Name, "flathub") {
			return true
		}
	}
	return false
}

// AddRemote adds a remote from a .flatpakrepo URL or repo URL.
func AddRemote(ctx context.Context, scope Scope, name, location string) error {
	args := append([]string{"remote-add", "--if-not-exists"}, scopeArgs(scope)...)
	args = append(args, name, location)
	_, err := run(ctx, args...)
	return err
}

// AddFlathub is shorthand for the standard Flathub remote.
func AddFlathub(ctx context.Context, scope Scope) error {
	return AddRemote(ctx, scope, "flathub", FlathubRepo)
}

// RemoveRemote deletes a remote. Scope selects --user/--system; ScopeAuto
// tries plain delete first, then user, then system to cover ambiguous setups.
func RemoveRemote(ctx context.Context, scope Scope, name string) error {
	if scope != ScopeAuto {
		args := append([]string{"remote-delete"}, scopeArgs(scope)...)
		args = append(args, name)
		_, err := run(ctx, args...)
		return err
	}
	if _, err := run(ctx, "remote-delete", name); err == nil {
		return nil
	}
	if _, err := run(ctx, "remote-delete", "--user", name); err == nil {
		return nil
	}
	_, err := run(ctx, "remote-delete", "--system", name)
	return err
}

// ListInstalled lists installed apps. appOnly=true adds --app (default UX);
// pass false to include runtimes (used by search toggle "both").
func ListInstalled(ctx context.Context, appOnly bool) ([]InstalledApp, error) {
	args := []string{"list", "--columns=application,name,origin,version,branch"}
	if appOnly {
		args = append(args, "--app")
	}
	out, err := run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var apps []InstalledApp
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		a := InstalledApp{}
		if len(parts) > 0 {
			a.ID = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			a.Name = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			a.Origin = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			a.Version = strings.TrimSpace(parts[3])
		}
		if len(parts) > 4 {
			a.Branch = strings.TrimSpace(parts[4])
		}
		if a.ID == "" {
			continue
		}
		apps = append(apps, a)
	}
	return apps, sc.Err()
}

// Search wraps `flatpak search` (offline fallback when Flathub API is down).
// kind: "app", "runtime", or "" for both.
func Search(ctx context.Context, query, kind string) ([]SearchResult, error) {
	args := []string{"search", "--columns=name,application,version,branch,remotes,description"}
	if kind == "app" {
		args = append(args, "--app")
	} else if kind == "runtime" {
		args = append(args, "--runtime")
	}
	args = append(args, query)
	out, err := run(ctx, args...)
	if err != nil {
		return nil, err
	}
	var hits []SearchResult
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		h := SearchResult{}
		if len(parts) > 0 {
			h.Name = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			h.AppID = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			h.Version = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			h.Branch = strings.TrimSpace(parts[3])
		}
		if len(parts) > 4 {
			h.Remotes = strings.TrimSpace(parts[4])
		}
		if len(parts) > 5 {
			h.Description = strings.TrimSpace(parts[5])
		}
		if h.AppID == "" {
			continue
		}
		hits = append(hits, h)
	}
	return hits, sc.Err()
}

// Install installs ref (optionally from a remote) with -y.
func Install(ctx context.Context, scope Scope, remote, ref string) error {
	args := []string{"install", "-y"}
	args = append(args, scopeArgs(scope)...)
	if remote != "" {
		args = append(args, remote)
	}
	args = append(args, ref)
	_, err := run(ctx, args...)
	return err
}

// Uninstall removes an installed ref with -y.
func Uninstall(ctx context.Context, scope Scope, ref string) error {
	args := []string{"uninstall", "-y"}
	args = append(args, scopeArgs(scope)...)
	args = append(args, ref)
	_, err := run(ctx, args...)
	return err
}

// Update updates one ref (or all when ref == "").
func Update(ctx context.Context, scope Scope, ref string) error {
	args := []string{"update", "-y"}
	args = append(args, scopeArgs(scope)...)
	if ref != "" {
		args = append(args, ref)
	}
	_, err := run(ctx, args...)
	return err
}

// Info returns `flatpak info` output for the details view.
func Info(ctx context.Context, ref string) (string, error) {
	return run(ctx, "info", ref)
}
