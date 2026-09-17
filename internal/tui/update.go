package tui

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/HuntedRaven7/tupak/internal/flathub"
	"github.com/HuntedRaven7/tupak/internal/flatpak"
)

// Update routes global keys first, then modal, then the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.sizeInputs()
		m.sizeDetail()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case remotesMsg:
		m.remBusy = false
		if msg.err != nil {
			m.remErr = msg.err.Error()
			return m, nil
		}
		m.remErr = ""
		m.remotes = msg.remotes
		if m.remCursor >= len(m.remotes) {
			m.remCursor = max(len(m.remotes)-1, 0)
		}
		// First-load Flathub check -> the requested popup.
		if m.modal == modalNone && !flatpak.HasFlathub(m.remotes) && m.status == "" {
			m.modal = modalFlathubMissing
			m.modalCursor = 0
		}
		return m, nil

	case appsMsg:
		m.appsBusy = false
		if msg.err != nil {
			m.appsErr = msg.err.Error()
			return m, nil
		}
		m.appsErr = ""
		m.apps = msg.apps
		return m, nil

	case installedMsg:
		m.installedBusy = false
		if msg.err != nil {
			m.installedErr = msg.err.Error()
			return m, nil
		}
		m.installedErr = ""
		m.installed = msg.apps
		return m, nil

	case searchMsg:
		m.searching = false
		if msg.err != nil {
			m.searchErr = msg.err.Error()
			return m, nil
		}
		m.searchErr = ""
		m.searchResults = msg.hits
		m.searchCursor = 0
		return m, nil

	case detailMsg:
		m.detailBusy = false
		if msg.err != nil {
			m.detailErr = msg.err.Error()
			return m, nil
		}
		m.detailErr = ""
		m.detail = msg.app
		m.detailView.SetContent(detailText(msg.app))
		m.detailView.GotoTop()
		return m, nil

	case doneMsg:
		m.status = msg.text
		m.remBusy = false
		// refresh whatever list is visible
		switch m.screen {
		case screenRemotes:
			cmds = append(cmds, loadRemotes())
			m.remBusy = true
		case screenInstalled:
			cmds = append(cmds, loadInstalled(true))
			m.installedBusy = true
		}
		return m, tea.Batch(cmds...)

	case failMsg:
		m.status = "error: " + msg.err.Error()
		m.remBusy = false
		m.appsBusy = false
		m.installedBusy = false
		m.searching = false
		m.detailBusy = false
		return m, nil
	}

	// Modal captures all keys while open.
	if m.modal != modalNone {
		return m.updateModal(msg)
	}

	// Global keys.
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c", "q":
			if m.screen == screenSearch && m.searchInput.Focused() {
				// let search screen handle q as text
				break
			}
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		case "1":
			return m.gotoScreen(screenHome)
		case "2":
			return m.gotoScreen(screenSearch)
		case "3":
			return m.gotoScreen(screenInstalled)
		case "4":
			return m.gotoScreen(screenRemotes)
		}
	}

	switch m.screen {
	case screenHome:
		return m.updateHome(msg)
	case screenCategory:
		return m.updateCategory(msg)
	case screenSearch:
		return m.updateSearch(msg)
	case screenDetails:
		return m.updateDetails(msg)
	case screenInstalled:
		return m.updateInstalled(msg)
	case screenRemotes:
		return m.updateRemotes(msg)
	}
	return m, nil
}

// gotoScreen switches tabs with a clean slate: any modal is dismissed,
// transient status/errors are cleared, inputs are blurred, scroll positions
// reset, and the target tab reloads its data — so the new tab never shows
// stale content from the previous one.
func (m Model) gotoScreen(s screen) (tea.Model, tea.Cmd) {
	m.screen = s
	m.modal = modalNone
	m.pending = opNone
	m.pendingRef = ""
	m.pendingArg = ""
	m.modalCursor = 0
	m.showHelp = false
	m.status = ""
	m.searchInput.Blur()
	m.addName.Blur()
	m.addURL.Blur()
	m.detailView.GotoTop()

	switch s {
	case screenHome:
		m.catCursor = 0
		if len(m.apps) == 0 && !m.appsBusy && m.appsErr == "" {
			m.appsBusy = true
			return m, loadApps()
		}
		return m, nil
	case screenSearch:
		m.searchCursor = 0
		m.searchErr = ""
		m.searching = false
		m.searchInput.Focus()
		return m, m.searchInput.Focus()
	case screenInstalled:
		m.installedCursor = 0
		m.installedErr = ""
		if !m.installedBusy {
			m.installedBusy = true
			return m, loadInstalled(true)
		}
		return m, nil
	case screenRemotes:
		m.remCursor = 0
		m.remErr = ""
		if !m.remBusy {
			m.remBusy = true
			return m, loadRemotes()
		}
		return m, nil
	}
	return m, nil
}

// --- home (categories) ---

func (m Model) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.apps) == 0 && !m.appsBusy && m.appsErr == "" {
		m.appsBusy = true
		return m, loadApps()
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.catCursor > 0 {
			m.catCursor--
		}
	case "down", "j":
		if m.catCursor < len(flathub.Categories)-1 {
			m.catCursor++
		}
	case "enter", "l", "right":
		m.activeCat = flathub.Categories[m.catCursor].Slug
		m.appsCursor = 0
		m.screen = screenCategory
		if len(m.apps) == 0 && !m.appsBusy {
			m.appsBusy = true
			return m, loadApps()
		}
	case "r":
		m.appsBusy = true
		return m, tea.Batch(m.spin.Tick, loadRemotes())
	}
	return m, nil
}

// --- category listing (apps-only) ---

func visibleApps(m Model) []flathub.App {
	apps := flathub.ByCategory(m.apps, m.activeCat)
	// apps-only default: drop anything that looks like a runtime ref.
	out := apps[:0]
	for _, a := range apps {
		if strings.HasSuffix(a.ID, ".Runtime") || strings.Contains(a.ID, ".Platform") {
			continue
		}
		out = append(out, a)
	}
	_ = out
	// NOTE: keep it simple — filter loosely by known runtime-ish IDs.
	filtered := make([]flathub.App, 0, len(apps))
	for _, a := range apps {
		l := strings.ToLower(a.ID)
		if strings.Contains(l, "runtime") || strings.HasSuffix(l, ".platform") || strings.HasSuffix(l, ".baseapp") {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered
}

func (m Model) updateCategory(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	list := visibleApps(m)
	switch key.String() {
	case "esc", "h", "left":
		m.screen = screenHome
	case "up", "k":
		if m.appsCursor > 0 {
			m.appsCursor--
		}
	case "down", "j":
		if m.appsCursor < len(list)-1 {
			m.appsCursor++
		}
	case "enter":
		if len(list) > 0 {
			m.detailRef = list[m.appsCursor].ID
			m.detail = list[m.appsCursor]
			m.screen = screenDetails
			m.detailBusy = true
			m.detailView.SetContent("Loading…")
			return m, loadDetail(m.detailRef)
		}
	case "i":
		if len(list) > 0 {
			m.pending = opInstall
			m.pendingRef = list[m.appsCursor].ID
			m.pendingArg = "flathub"
			m.modal = modalScope
			m.modalCursor = 0 // default to --user
		}
	}
	return m, nil
}

// --- search (toggle Apps / Runtimes / Both) ---

func (m Model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	// When input is focused, most keys go to it.
	if m.searchInput.Focused() {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case "enter":
				m.searchInput.Blur()
				m.searching = true
				return m, runSearch(m.searchInput.Value(), m.searchType)
			case "esc":
				m.searchInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "/", "i":
		m.searchInput.Focus()
		return m, m.searchInput.Focus()
	case "tab", "t":
		// cycle Apps -> Runtimes -> Both
		switch m.searchType {
		case flathub.SearchApps:
			m.searchType = flathub.SearchRuntimes
		case flathub.SearchRuntimes:
			m.searchType = flathub.SearchBoth
		default:
			m.searchType = flathub.SearchApps
		}
		if strings.TrimSpace(m.searchInput.Value()) != "" {
			m.searching = true
			return m, runSearch(m.searchInput.Value(), m.searchType)
		}
	case "enter":
		if strings.TrimSpace(m.searchInput.Value()) != "" {
			m.searching = true
			return m, runSearch(m.searchInput.Value(), m.searchType)
		}
	case "up", "k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
	case "down", "j":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
	case "d":
		if len(m.searchResults) > 0 {
			m.detailRef = m.searchResults[m.searchCursor].AppID
			m.screen = screenDetails
			m.detailBusy = true
			m.detailView.SetContent("Loading…")
			return m, loadDetail(m.detailRef)
		}
	}
	return m, nil
}

// --- details ---

func (m Model) updateDetails(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.detailView, cmd = m.detailView.Update(msg)
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "esc", "h", "left":
			m.screen = screenCategory
			if m.detailRef == "" {
				m.screen = screenHome
			}
			return m, nil
		case "i":
			m.pending = opInstall
			m.pendingRef = m.detailRef
			if m.pendingRef == "" {
				m.pendingRef = m.detail.ID
			}
			m.pendingArg = "flathub"
			m.modal = modalScope
			m.modalCursor = 0
			return m, nil
		case "x", "u":
			m.pending = opUninstall
			m.pendingRef = m.detail.ID
			if m.pendingRef == "" {
				m.pendingRef = m.detailRef
			}
			m.modal = modalScope
			m.modalCursor = 0
			return m, nil
		}
	}
	return m, cmd
}

// --- installed (apps-only) ---

func (m Model) updateInstalled(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.installed) == 0 && !m.installedBusy && m.installedErr == "" {
		m.installedBusy = true
		return m, loadInstalled(true)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "r":
		m.installedBusy = true
		return m, loadInstalled(true)
	case "up", "k":
		if m.installedCursor > 0 {
			m.installedCursor--
		}
	case "down", "j":
		if m.installedCursor < len(m.installed)-1 {
			m.installedCursor++
		}
	case "x", "d":
		if len(m.installed) > 0 {
			ref := m.installed[m.installedCursor].ID
			m.pending = opUninstall
			m.pendingRef = ref
			m.confirmText = "Uninstall " + ref + "?"
			m.modal = modalConfirm
			m.modalCursor = 1 // default No
		}
	case "u":
		if len(m.installed) > 0 {
			m.pending = opUpdateOne
			m.pendingRef = m.installed[m.installedCursor].ID
			m.modal = modalScope
			m.modalCursor = 0
		}
	case "U":
		m.pending = opUpdateAll
		m.pendingRef = ""
		m.modal = modalScope
		m.modalCursor = 0
	case "enter":
		if len(m.installed) > 0 {
			m.detailRef = m.installed[m.installedCursor].ID
			m.screen = screenDetails
			m.detailBusy = true
			m.detailView.SetContent("Loading…")
			return m, loadDetail(m.detailRef)
		}
	}
	return m, nil
}

// --- remotes ---

func (m Model) updateRemotes(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.remotes) == 0 && !m.remBusy && m.remErr == "" {
		m.remBusy = true
		return m, loadRemotes()
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "r":
		m.remBusy = true
		return m, loadRemotes()
	case "up", "k":
		if m.remCursor > 0 {
			m.remCursor--
		}
	case "down", "j":
		if m.remCursor < len(m.remotes)-1 {
			m.remCursor++
		}
	case "a":
		m.addName.Reset()
		m.addURL.Reset()
		m.addFocus = 0
		m.addName.Focus()
		m.modal = modalAddRemote
		return m, m.addName.Focus()
	case "d", "x", "delete":
		if len(m.remotes) > 0 {
			name := m.remotes[m.remCursor].Name
			m.pending = opRemoveRemote
			m.pendingRef = name
			m.confirmText = "Remove remote '" + name + "'?"
			m.modal = modalConfirm
			m.modalCursor = 1 // default No for safety
		}
	}
	return m, nil
}

// --- modal ---

func (m Model) updateModal(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Add-remote modal has two text inputs.
	if m.modal == modalAddRemote {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.modal = modalNone
				m.addName.Blur()
				m.addURL.Blur()
				return m, nil
			case "tab", "down", "up":
				if m.addFocus == 0 {
					m.addFocus = 1
					m.addName.Blur()
					m.addURL.Focus()
					return m, m.addURL.Focus()
				}
				m.addFocus = 0
				m.addURL.Blur()
				m.addName.Focus()
				return m, m.addName.Focus()
			case "enter":
				name := strings.TrimSpace(m.addName.Value())
				url := strings.TrimSpace(m.addURL.Value())
				if name == "" || url == "" {
					m.status = "name and url are both required"
					return m, nil
				}
				m.addName.Blur()
				m.addURL.Blur()
				m.pending = opAddRemote
				m.pendingRef = name
				m.pendingArg = url
				m.modal = modalScope
				m.modalCursor = 0
				return m, nil
			}
		}
		var c1, c2 tea.Cmd
		if m.addFocus == 0 {
			m.addName, c1 = m.addName.Update(msg)
		} else {
			m.addURL, c2 = m.addURL.Update(msg)
		}
		return m, tea.Batch(c1, c2)
	}

	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	k := key.String()

	switch m.modal {
	case modalFlathubMissing:
		// [Add Flathub / Add other / Skip]
		switch k {
		case "left", "h", "up", "k":
			if m.modalCursor > 0 {
				m.modalCursor--
			}
		case "right", "l", "down", "j", "tab":
			if m.modalCursor < 2 {
				m.modalCursor++
			}
		case "enter":
			switch m.modalCursor {
			case 0:
				m.pending = opAddFlathub
				m.modal = modalScope
				m.modalCursor = 0
			case 1:
				m.addName.Reset()
				m.addURL.Reset()
				m.addFocus = 0
				m.addName.Focus()
				m.modal = modalAddRemote
				return m, m.addName.Focus()
			default:
				m.modal = modalNone
				m.status = "skipped Flathub setup — press 4 to manage remotes"
			}
		case "esc":
			m.modal = modalNone
		}

	case modalConfirm:
		switch k {
		case "left", "h", "right", "l", "tab":
			m.modalCursor = 1 - m.modalCursor
		case "enter":
			if m.modalCursor == 0 { // Yes
				m.modal = modalNone
				if m.pending == opRemoveRemote {
					// scope matters for delete: ask via scope picker instead
					op := m.pending
					_ = op
					m.modal = modalScope
					m.modalCursor = 0
					return m, nil
				}
				return m, m.execPending(flatpak.ScopeAuto)
			}
			m.modal = modalNone
			m.pending = opNone
		case "esc":
			m.modal = modalNone
			m.pending = opNone
		case "y":
			m.modal = modalNone
			if m.pending == opRemoveRemote {
				m.modal = modalScope
				m.modalCursor = 0
				return m, nil
			}
			return m, m.execPending(flatpak.ScopeAuto)
		case "n":
			m.modal = modalNone
			m.pending = opNone
		}

	case modalScope:
		// [user / system], user default
		switch k {
		case "left", "h", "right", "l", "tab", "up", "down":
			m.modalCursor = 1 - m.modalCursor
		case "enter":
			scope := flatpak.ScopeUser
			if m.modalCursor == 1 {
				scope = flatpak.ScopeSystem
			}
			m.modal = modalNone
			cmd := m.execPending(scope)
			m.pending = opNone
			if cmd == nil {
				return m, loadRemotes()
			}
			return m, cmd
		case "esc":
			m.modal = modalNone
			m.pending = opNone
		case "u":
			m.modal = modalNone
			cmd := m.execPending(flatpak.ScopeUser)
			m.pending = opNone
			return m, cmd
		case "s":
			m.modal = modalNone
			cmd := m.execPending(flatpak.ScopeSystem)
			m.pending = opNone
			return m, cmd
		}
	}
	return m, nil
}

// execPending maps the stored pending op + chosen scope to a tea.Cmd.
func (m Model) execPending(scope flatpak.Scope) tea.Cmd {
	switch m.pending {
	case opAddFlathub:
		mRemBusy := true
		_ = mRemBusy
		return doAddFlathub(scope)
	case opAddRemote:
		return doAddRemote(scope, m.pendingRef, m.pendingArg)
	case opRemoveRemote:
		return doRemoveRemote(scope, m.pendingRef)
	case opInstall:
		return doInstall(scope, m.pendingArg, m.pendingRef)
	case opUninstall:
		// confirm first when coming straight from a scope-less flow
		return doUninstall(scope, m.pendingRef)
	case opUpdateOne:
		return doUpdate(scope, m.pendingRef)
	case opUpdateAll:
		return doUpdate(scope, "")
	default:
		return nil
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
