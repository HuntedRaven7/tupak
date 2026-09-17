package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/HuntedRaven7/tupak/internal/flathub"
)

// View renders the active screen pinned to the full terminal size, plus any
// modal overlay (centered on a cleared screen).
func (m Model) View() tea.View {
	w, h := m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	if m.modal != modalNone {
		return tea.NewView(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, m.modalView()))
	}

	header := m.header()
	footer := m.footer()
	// Body gets everything between header, the blank separator line and footer.
	bodyH := h - lipgloss.Height(header) - lipgloss.Height(footer) - 1
	if bodyH < 3 {
		bodyH = 3
	}
	// Trim trailing newlines so they don't become phantom rows; Place then
	// pads the body to exactly bodyH lines, pinning the footer at the bottom.
	body := lipgloss.Place(w, bodyH, lipgloss.Left, lipgloss.Top, strings.TrimRight(m.body(bodyH), "\n"))
	return tea.NewView(header + "\n\n" + body + "\n" + footer)
}

func (m Model) header() string {
	tabs := []string{"1 Home", "2 Search", "3 Installed", "4 Remotes"}
	active := map[screen]int{
		screenHome: 0, screenCategory: 0, screenDetails: 0,
		screenSearch: 1, screenInstalled: 2, screenRemotes: 3,
	}[m.screen]
	var parts []string
	for i, t := range tabs {
		if i == active {
			parts = append(parts, m.st.selected.Render("["+t+"]"))
		} else {
			parts = append(parts, m.st.dim.Render(t))
		}
	}
	return m.st.title.Render("tupak") + "  " + strings.Join(parts, "  ")
}

func (m Model) body(maxRows int) string {
	switch m.screen {
	case screenHome:
		return m.homeView(maxRows)
	case screenCategory:
		return m.categoryView(maxRows)
	case screenSearch:
		return m.searchView(maxRows)
	case screenDetails:
		return m.detailsView()
	case screenInstalled:
		return m.installedView(maxRows)
	case screenRemotes:
		return m.remotesView(maxRows)
	}
	return ""
}

func (m Model) footer() string {
	if m.showHelp {
		return m.st.status.Render(m.helpText())
	}
	keys := ""
	switch m.screen {
	case screenHome:
		keys = "up/down navigate • enter open • 1-4 screens • ? help • q quit"
	case screenCategory:
		keys = "up/down navigate • enter details • i install (pick user/system) • esc back"
	case screenSearch:
		keys = "/ focus • enter search • tab toggle Apps/Runtimes/Both • d details • esc blur"
	case screenDetails:
		keys = "i install • x uninstall • esc back • scroll with up/down"
	case screenInstalled:
		keys = "enter details • x uninstall • u update • U update all • r refresh"
	case screenRemotes:
		keys = "a add • d remove • r refresh"
	}
	status := ""
	if strings.TrimSpace(m.status) != "" {
		status = "  •  " + m.status
	}
	busy := ""
	if m.remBusy || m.appsBusy || m.installedBusy || m.searching || m.detailBusy {
		busy = "  " + m.spin.View() + " working…"
	}
	return m.st.status.Render(keys+status+busy)
}

func (m Model) helpText() string {
	return "keys: 1 home • 2 search • 3 installed • 4 remotes • / search • tab type • i install • x/d delete • u update • a add remote • ? toggle help • q quit"
}

// --- screens ---

func (m Model) homeView(maxRows int) string {
	var b strings.Builder
	b.WriteString(m.st.subtitle.Render("Categories — Bazaar-style browsing (apps-only)") + "\n\n")
	if m.appsBusy && len(m.apps) == 0 {
		b.WriteString(m.spin.View() + " loading Flathub catalog…\n")
		return b.String()
	}
	if m.appsErr != "" {
		b.WriteString(m.st.err.Render("catalog: "+m.appsErr) + "\n")
	}
	rows := max(maxRows-2, 3)
	if m.appsErr != "" {
		rows = max(rows-1, 3)
	}
	start, end := window(m.catCursor, len(flathub.Categories), rows)
	for i := start; i < end; i++ {
		c := flathub.Categories[i]
		n := len(flathub.ByCategory(m.apps, c.Slug))
		line := m.fit(fmt.Sprintf("[%s] %s (%d)", c.Icon, c.Label, n))
		if i == m.catCursor {
			b.WriteString(m.st.selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

func (m Model) categoryView(maxRows int) string {
	var b strings.Builder
	label := flathub.LabelFor(m.activeCat)
	b.WriteString(m.st.subtitle.Render(label+" (apps-only, i to install)") + "\n\n")
	if m.appsBusy {
		b.WriteString(m.spin.View() + " loading…\n")
		return b.String()
	}
	list := visibleApps(m)
	if len(list) == 0 {
		b.WriteString(m.st.dim.Render("No apps in this category yet (or offline).") + "\n")
		return b.String()
	}
	rows := max(maxRows-4, 3) // subtitle block + summary footer
	start, end := window(m.appsCursor, len(list), rows)
	for i := start; i < end; i++ {
		a := list[i]
		line := m.fit(fmt.Sprintf("%s — %s", a.DisplayName(), firstLine(a.Summary)))
		if i == m.appsCursor {
			b.WriteString(m.st.selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString(m.st.dim.Render(fmt.Sprintf("\n%d apps • showing %d-%d", len(list), start+1, end)))
	return b.String()
}

func (m Model) searchView(maxRows int) string {
	var b strings.Builder
	b.WriteString(m.st.subtitle.Render(fmt.Sprintf("Search Flathub [%s] — tab toggles Apps/Runtimes/Both", searchTypeLabel(m.searchType))) + "\n\n")
	b.WriteString(m.searchInput.View() + "\n\n")
	if m.searching {
		b.WriteString(m.spin.View() + " searching…\n")
		return b.String()
	}
	if m.searchErr != "" {
		b.WriteString(m.st.err.Render(m.searchErr) + "\n")
	}
	if len(m.searchResults) == 0 {
		b.WriteString(m.st.dim.Render("Type a query, press enter. `flatpak search` is used if the API is down.") + "\n")
		return b.String()
	}
	rows := max(maxRows-4, 3) // subtitle block + input block
	start, end := window(m.searchCursor, len(m.searchResults), rows)
	for i := start; i < end; i++ {
		h := m.searchResults[i]
		tag := ""
		if h.IsRuntime {
			tag = " [runtime]"
		}
		line := m.fit(fmt.Sprintf("%s (%s)%s — %s", h.Name, h.AppID, tag, firstLine(h.Summary)))
		if i == m.searchCursor {
			b.WriteString(m.st.selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

func (m Model) detailsView() string {
	var b strings.Builder
	title := m.detail.DisplayName()
	if title == "" {
		title = m.detailRef
	}
	b.WriteString(m.st.subtitle.Render(title) + "\n\n")
	if m.detailBusy {
		b.WriteString(m.spin.View() + " loading details…\n")
		return b.String()
	}
	if m.detailErr != "" {
		b.WriteString(m.st.err.Render(m.detailErr) + "\n")
	}
	b.WriteString(m.detailView.View())
	b.WriteString("\n" + m.st.dim.Render("i install (user/system) • x uninstall • esc back"))
	return b.String()
}

func detailText(a flathub.App) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", a.ID)
	if a.Developer != "" {
		fmt.Fprintf(&b, "By: %s\n", a.Developer)
	}
	if a.Version != "" {
		fmt.Fprintf(&b, "Version: %s\n", a.Version)
	}
	if a.License != "" {
		fmt.Fprintf(&b, "License: %s\n", a.License)
	}
	if len(a.Categories) > 0 {
		fmt.Fprintf(&b, "Categories: %s\n", strings.Join(a.Categories, ", "))
	}
	if a.Homepage != "" {
		fmt.Fprintf(&b, "Home: %s\n", a.Homepage)
	}
	if a.Bugtracker != "" {
		fmt.Fprintf(&b, "Issues: %s\n", a.Bugtracker)
	}
	if a.Summary != "" {
		fmt.Fprintf(&b, "\n%s\n", a.Summary)
	}
	if a.Description != "" {
		desc := strings.TrimSpace(stripHTML(a.Description))
		if len(desc) > 2000 {
			desc = desc[:2000] + "…"
		}
		fmt.Fprintf(&b, "\n%s\n", desc)
	}
	return b.String()
}

func (m Model) installedView(maxRows int) string {
	var b strings.Builder
	b.WriteString(m.st.subtitle.Render("Installed (apps-only)") + "\n\n")
	if m.installedBusy && len(m.installed) == 0 {
		b.WriteString(m.spin.View() + " loading `flatpak list`…\n")
		return b.String()
	}
	if m.installedErr != "" {
		b.WriteString(m.st.err.Render(m.installedErr) + "\n")
	}
	if len(m.installed) == 0 {
		b.WriteString(m.st.dim.Render("Nothing installed (or flatpak not found).") + "\n")
		return b.String()
	}
	rows := max(maxRows-2, 3)
	start, end := window(m.installedCursor, len(m.installed), rows)
	for i := start; i < end; i++ {
		a := m.installed[i]
		line := m.fit(fmt.Sprintf("%s (%s) %s [%s]", a.Name, a.ID, a.Version, a.Origin))
		if i == m.installedCursor {
			b.WriteString(m.st.selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

func (m Model) remotesView(maxRows int) string {
	var b strings.Builder
	b.WriteString(m.st.subtitle.Render("Flatpak remotes — a add • d remove • r refresh") + "\n\n")
	if m.remBusy && len(m.remotes) == 0 {
		b.WriteString(m.spin.View() + " loading `flatpak remotes`…\n")
		return b.String()
	}
	if m.remErr != "" {
		b.WriteString(m.st.err.Render(m.remErr) + "\n")
	}
	if len(m.remotes) == 0 {
		b.WriteString(m.st.dim.Render("No remotes configured.") + "\n")
		return b.String()
	}
	rows := max(maxRows-2, 3)
	start, end := window(m.remCursor, len(m.remotes), rows)
	for i := start; i < end; i++ {
		r := m.remotes[i]
		scope := "user"
		if r.IsSystem() {
			scope = "system"
		}
		line := m.fit(fmt.Sprintf("%s  %s  [%s]", r.Name, r.URL, scope))
		if i == m.remCursor {
			b.WriteString(m.st.selected.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// --- modal ---

func (m Model) modalView() string {
	var inner string
	switch m.modal {
	case modalFlathubMissing:
		opts := []string{"Add Flathub", "Add other", "Skip"}
		var parts []string
		for i, o := range opts {
			if i == m.modalCursor {
				parts = append(parts, m.st.selected.Render("["+o+"]"))
			} else {
				parts = append(parts, " "+o+" ")
			}
		}
		inner = "You don't have a remote flatpak store.\nWould you like to add Flathub's remote?\n\n" + strings.Join(parts, "  ") + "\n\nenter select • esc dismiss"
	case modalConfirm:
		yes, no := " Yes ", " No "
		if m.modalCursor == 0 {
			yes = m.st.selected.Render("[Yes]")
		}
		if m.modalCursor == 1 {
			no = m.st.selected.Render("[No]")
		}
		inner = m.confirmText + "\n\n" + yes + "  " + no + "\n\ny/n or enter • esc cancel"
	case modalScope:
		user, sys := " --user ", " --system "
		if m.modalCursor == 0 {
			user = m.st.selected.Render("[--user]")
		}
		if m.modalCursor == 1 {
			sys = m.st.selected.Render("[--system]")
		}
		action := pendingLabel(m.pending, m.pendingRef)
		inner = action + "\nInstall scope?\n\n" + user + "  " + sys + "\n\nenter select • u user • s system • esc cancel"
	case modalAddRemote:
		inner = "Add remote\n\n" + m.addName.View() + "\n" + m.addURL.View() + "\n\ntab switch field • enter continue • esc cancel"
	}
	return m.st.modal.Render(inner)
}

func pendingLabel(op pendingOp, ref string) string {
	switch op {
	case opAddFlathub:
		return "Add Flathub remote"
	case opAddRemote:
		return "Add remote '" + ref + "'"
	case opRemoveRemote:
		return "Remove remote '" + ref + "'"
	case opInstall:
		return "Install '" + ref + "'"
	case opUninstall:
		return "Uninstall '" + ref + "'"
	case opUpdateOne:
		return "Update '" + ref + "'"
	case opUpdateAll:
		return "Update everything"
	}
	return "Confirm"
}

// --- helpers ---

// fit truncates a plain (unstyled) line to the terminal width so long rows
// never wrap and push the footer off-screen. Call before styling.
func (m Model) fit(s string) string {
	w := m.width - 4 // cursor prefix + margin
	if m.width <= 0 {
		w = defaultWidth - 4
	}
	r := []rune(strings.TrimRight(s, "\n"))
	if len(r) <= w {
		return string(r)
	}
	if w <= 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func window(cursor, total, size int) (int, int) {
	if total <= size {
		return 0, total
	}
	start := cursor - size/2
	if start < 0 {
		start = 0
	}
	if start+size > total {
		start = total - size
	}
	return start, start + size
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func searchTypeLabel(t flathub.SearchType) string {
	switch t {
	case flathub.SearchRuntimes:
		return "Runtimes"
	case flathub.SearchBoth:
		return "Both"
	default:
		return "Apps"
	}
}

// stripHTML drops simple tags from AppStream descriptions.
func stripHTML(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out.WriteRune(r)
		}
	}
	return out.String()
}
