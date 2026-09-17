package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The view must exactly fill the terminal at any size so the footer stays
// pinned and lists use all available rows.
func TestViewFillsTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {60, 15}, {200, 60}} {
		m := New()
		m.width, m.height = size[0], size[1]
		m.sizeInputs()
		m.sizeDetail()
		for _, s := range []screen{screenHome, screenCategory, screenSearch, screenInstalled, screenRemotes} {
			m.screen = s
			v := m.View()
			// Content is the styled screen text; a trailing newline moves
			// the cursor but doesn't consume a row, so trim it to count.
			lines := len(strings.Split(strings.TrimRight(v.Content, "\n"), "\n"))
			if lines != size[1] {
				t.Fatalf("screen %d at %dx%d: got %d lines, want %d", s, size[0], size[1], lines, size[1])
			}
		}
	}
}

// Switching tabs must reset transient state so no stale content shows.
func TestGotoScreenResets(t *testing.T) {
	m := New()
	m.status = "old"
	m.modal = modalConfirm
	m.searchErr = "old err"
	m.searchCursor = 5
	m.remCursor = 3

	got, _ := m.gotoScreen(screenSearch)
	g := got.(Model)
	if g.modal != modalNone || g.status != "" || g.searchErr != "" || g.searchCursor != 0 {
		t.Fatalf("search tab not reset: %+v", g)
	}
	if !g.searchInput.Focused() {
		t.Fatalf("search input should be focused on entering search tab")
	}

	got2, _ := m.gotoScreen(screenRemotes)
	r := got2.(Model)
	if r.remCursor != 0 || !r.remBusy {
		t.Fatalf("remotes tab should reset cursor and reload, got %+v", r)
	}
	var _ tea.Model = New()
}
