package flathub

import (
	"encoding/json"
	"testing"
)

func TestAppUnmarshalNoRecursion(t *testing.T) {
	raw := `{"id":"org.gnome.gedit","name":"gedit","summary":"edit","developer_name":"GNOME","project_license":"GPL","urls":{"homepage":"https://example.com","bugtracker":"https://bugs.example.com"}}`
	var a App
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ID != "org.gnome.gedit" || a.Homepage != "https://example.com" || a.Bugtracker != "https://bugs.example.com" {
		t.Fatalf("bad decode: %+v", a)
	}
	var apps []App
	bulk := `[` + raw + `,` + raw + `]`
	if err := json.Unmarshal([]byte(bulk), &apps); err != nil {
		t.Fatalf("bulk unmarshal: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("want 2, got %d", len(apps))
	}
}

// Search/collection hits carry main_categories as a single lowercase string.
func TestAppUnmarshalStringCategory(t *testing.T) {
	raw := `{"app_id":"org.vinegarhq.Sober","name":"Sober","summary":"roblox","developer_name":"VinegarHQ","project_license":"proprietary","main_categories":"game","sub_categories":["GNOME"],"type":"desktop-application"}`
	var a App
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.ID != "org.vinegarhq.Sober" {
		t.Fatalf("bad id: %+v", a)
	}
	found := false
	for _, c := range a.Categories {
		if c == "game" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing main category, got: %v", a.Categories)
	}
}

// Detail objects use `categories` array + releases[0].version.
func TestAppUnmarshalDetailShape(t *testing.T) {
	raw := `{"id":"org.gnome.gedit","name":"gedit","summary":"edit","categories":["Utility","TextEditor"],"keywords":["Text"],"releases":[{"version":"50.0"}]}`
	var a App
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a.Version != "50.0" {
		t.Fatalf("bad version: %+v", a)
	}
	if len(a.Categories) != 2 || a.Categories[0] != "Utility" {
		t.Fatalf("bad categories: %v", a.Categories)
	}
	if got := len(ByCategory([]App{a}, "utility")); got != 1 {
		t.Fatalf("ByCategory case-insensitive match failed")
	}
}
