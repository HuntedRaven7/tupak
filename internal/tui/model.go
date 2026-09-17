package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/HuntedRaven7/tupak/internal/flathub"
	"github.com/HuntedRaven7/tupak/internal/flatpak"
)

// screen is the active top-level view.
type screen int

const (
	screenHome screen = iota
	screenCategory
	screenSearch
	screenDetails
	screenInstalled
	screenRemotes
)

// modalKind overlays the current screen.
type modalKind int

const (
	modalNone modalKind = iota
	modalFlathubMissing
	modalConfirm
	modalScope
	modalAddRemote
)

// pendingOp remembers what a scope pick (or confirm) should execute.
type pendingOp int

const (
	opNone pendingOp = iota
	opAddFlathub
	opAddRemote
	opRemoveRemote
	opInstall
	opUninstall
	opUpdateOne
	opUpdateAll
)

// Model is the root Elm model; sub-screens are cursor+slice fields
// (kept flat on purpose so global keys stay trivial).
type Model struct {
	screen screen
	width  int
	height int
	st     styles

	spin spinner.Model

	// data
	remotes   []flatpak.Remote
	remCursor int
	remErr    string
	remBusy   bool

	catCursor  int
	apps       []flathub.App
	appsCursor int
	appsBusy   bool
	appsErr    string
	activeCat  string

	installed       []flatpak.InstalledApp
	installedCursor int
	installedBusy   bool
	installedErr    string

	searchInput   textinput.Model
	searchResults []flathub.Hit
	searchCursor  int
	searchType    flathub.SearchType
	searching     bool
	searchErr     string

	detail     flathub.App
	detailBusy bool
	detailErr  string
	detailRef  string
	detailView viewport.Model

	// modal state
	modal       modalKind
	modalCursor int
	confirmText string
	pending     pendingOp
	pendingRef  string // app id or remote name
	pendingArg  string // extra: remote URL for opAddRemote

	addName textinput.Model
	addURL  textinput.Model
	addFocus int // 0=name, 1=url

	status string
	showHelp bool
}

// Sensible fallback size until the first WindowSizeMsg arrives.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// New builds the initial model.
func New() Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	si := textinput.New()
	si.Placeholder = "Search apps… (/ to focus, enter to search)"
	si.Prompt = "/ "
	si.CharLimit = 128

	an := textinput.New()
	an.Placeholder = "name (e.g. flathub)"
	an.Prompt = "name: "
	an.CharLimit = 64
	au := textinput.New()
	au.Placeholder = "https://…/….flatpakrepo"
	au.Prompt = "url:  "
	au.CharLimit = 256

	vp := viewport.New()

	// New builds the initial model.
	m := Model{
		screen:      screenHome,
		width:       defaultWidth,
		height:      defaultHeight,
		st:          defaultStyles(),
		spin:        sp,
		searchInput: si,
		searchType:  flathub.SearchApps,
		activeCat:   "All",
		addName:     an,
		addURL:      au,
		detailView:  vp,
	}
	m.sizeInputs()
	m.sizeDetail()
	return m
}

// sizeInputs keeps text inputs within the terminal width.
func (m *Model) sizeInputs() {
	m.searchInput.SetWidth(max(m.width-12, 20))
	m.addName.SetWidth(max(m.width-20, 20))
	m.addURL.SetWidth(max(m.width-20, 20))
}

// sizeDetail keeps the details viewport within the terminal.
func (m *Model) sizeDetail() {
	m.detailView.SetWidth(max(m.width-8, 20))
	m.detailView.SetHeight(max(m.height-12, 5))
}

// Init loads remotes first so we can pop the Flathub-missing modal.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, loadRemotes())
}

// --- messages ---

type remotesMsg struct {
	remotes []flatpak.Remote
	err     error
}

type appsMsg struct {
	apps []flathub.App
	err  error
}

type installedMsg struct {
	apps []flatpak.InstalledApp
	err  error
}

type searchMsg struct {
	hits []flathub.Hit
	err  error
}

type detailMsg struct {
	app flathub.App
	err error
}

type doneMsg struct{ text string }

type failMsg struct{ err error }

// --- commands (all async, Update never blocks) ---

func loadRemotes() tea.Cmd {
	return func() tea.Msg {
		r, err := flatpak.ListRemotes(context.Background())
		return remotesMsg{remotes: r, err: err}
	}
}

func loadApps() tea.Cmd {
	return func() tea.Msg {
		a, err := flathub.FetchAll(context.Background())
		return appsMsg{apps: a, err: err}
	}
}

func loadInstalled(appOnly bool) tea.Cmd {
	return func() tea.Msg {
		a, err := flatpak.ListInstalled(context.Background(), appOnly)
		return installedMsg{apps: a, err: err}
	}
}

func runSearch(query string, typ flathub.SearchType) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(query) == "" {
			return searchMsg{}
		}
		hits, err := flathub.Search(context.Background(), query, typ, 30)
		if err == nil {
			return searchMsg{hits: hits}
		}
		// Offline fallback: flatpak search (apps-only unless toggle says otherwise).
		kind := ""
		switch typ {
		case flathub.SearchApps:
			kind = "app"
		case flathub.SearchRuntimes:
			kind = "runtime"
		}
		fb, fbErr := flatpak.Search(context.Background(), query, kind)
		if fbErr != nil {
			return searchMsg{err: err}
		}
		out := make([]flathub.Hit, 0, len(fb))
		for _, h := range fb {
			out = append(out, flathub.Hit{
				AppID:   h.AppID,
				Name:    h.Name,
				Summary: h.Description,
			})
		}
		return searchMsg{hits: out}
	}
}

func loadDetail(appID string) tea.Cmd {
	return func() tea.Msg {
		a, err := flathub.Get(context.Background(), appID)
		return detailMsg{app: a, err: err}
	}
}

func doInstall(scope flatpak.Scope, remote, ref string) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.Install(context.Background(), scope, remote, ref); err != nil {
			return failMsg{err}
		}
		return doneMsg{fmt.Sprintf("installed %s (%s)", ref, scopeLabel(scope))}
	}
}

func doUninstall(scope flatpak.Scope, ref string) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.Uninstall(context.Background(), scope, ref); err != nil {
			return failMsg{err}
		}
		return doneMsg{fmt.Sprintf("uninstalled %s (%s)", ref, scopeLabel(scope))}
	}
}

func doUpdate(scope flatpak.Scope, ref string) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.Update(context.Background(), scope, ref); err != nil {
			return failMsg{err}
		}
		if ref == "" {
			return doneMsg{fmt.Sprintf("updated everything (%s)", scopeLabel(scope))}
		}
		return doneMsg{fmt.Sprintf("updated %s (%s)", ref, scopeLabel(scope))}
	}
}

func doAddRemote(scope flatpak.Scope, name, url string) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.AddRemote(context.Background(), scope, name, url); err != nil {
			return failMsg{err}
		}
		return doneMsg{fmt.Sprintf("added remote %s (%s)", name, scopeLabel(scope))}
	}
}

func doAddFlathub(scope flatpak.Scope) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.AddFlathub(context.Background(), scope); err != nil {
			return failMsg{err}
		}
		return doneMsg{fmt.Sprintf("added Flathub (%s)", scopeLabel(scope))}
	}
}

func doRemoveRemote(scope flatpak.Scope, name string) tea.Cmd {
	return func() tea.Msg {
		if err := flatpak.RemoveRemote(context.Background(), scope, name); err != nil {
			return failMsg{err}
		}
		return doneMsg{fmt.Sprintf("removed remote %s", name)}
	}
}

func scopeLabel(s flatpak.Scope) string {
	if s == flatpak.ScopeSystem {
		return "system"
	}
	if s == flatpak.ScopeUser {
		return "user"
	}
	return "auto"
}
