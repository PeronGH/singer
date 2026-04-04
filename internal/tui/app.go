package tui

import (
	"io"
	"net/http"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/PeronGH/singer/internal/store"
	"github.com/PeronGH/singer/sub"
)

type appState int

const (
	stateBrowsing appState = iota
	stateInputting
	stateRunning
)

const helpBarH = 1

type App struct {
	store  *store.Store
	groups []group
	tab    int
	state  appState

	input   inputModel
	runner  runner
	logView viewport.Model

	width, height int
	err           error
}

func NewApp(s *store.Store) *App {
	groups, _ := s.LoadGroups()
	tabs := make([]group, len(groups))
	for i, g := range groups {
		tabs[i] = newGroup(g, 0, 0)
	}
	return &App{
		store:   s,
		groups:  tabs,
		logView: viewport.New(),
	}
}

func (a *App) Init() tea.Cmd {
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return a, a.handleResize(msg)
	case logLineMsg:
		return a, a.handleLogLine(msg)
	case runnerDoneMsg:
		return a, a.handleRunnerDone(msg)
	case fetchDoneMsg:
		return a, a.handleFetchDone(msg)
	case inputResultMsg:
		return a, a.handleInputResult(msg)
	case inputCancelMsg:
		a.state = stateBrowsing
		return a, nil
	case connectMsg:
		return a, a.handleConnect(msg)
	case addNodeMsg:
		return a, a.startAddNode()
	case editNodeMsg:
		return a, a.startEditNode(msg)
	case deleteNodeMsg:
		return a, a.handleDeleteNode(msg)
	case refreshMsg:
		return a, a.handleRefresh()
	case tea.PasteMsg:
		if a.state == stateInputting {
			return a, a.input.Update(msg)
		}
	case tea.KeyPressMsg:
		return a, a.handleKey(msg)
	}

	// Forward unhandled messages to active list.
	if a.state == stateBrowsing && len(a.groups) > 0 {
		return a, a.updateActiveList(msg)
	}
	return a, nil
}

func (a *App) handleResize(msg tea.WindowSizeMsg) tea.Cmd {
	a.width = msg.Width
	a.height = msg.Height
	a.resizeChildren()
	return nil
}

func (a *App) resizeChildren() {
	listH := a.listHeight()
	for i := range a.groups {
		a.groups[i].setSize(a.width, listH)
	}
	logH := a.logPaneHeight()
	a.logView.SetWidth(a.width)
	a.logView.SetHeight(logH)
}

func (a *App) listHeight() int {
	h := a.height - helpBarH
	if a.state == stateRunning || len(a.runner.lines) > 0 {
		h -= a.logPaneHeight() + 1 // +1 for header
	}
	if a.state == stateInputting {
		h -= 1
	}
	if a.err != nil {
		h -= 1
	}
	return max(h, 3)
}

func (a *App) logPaneHeight() int {
	return max(a.height/3, 5)
}

func (a *App) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()

	// ctrl+c always works, regardless of state.
	if key == "ctrl+c" {
		if a.state == stateRunning {
			a.runner.stop()
			a.state = stateBrowsing
			return nil
		}
		if a.state == stateInputting {
			a.state = stateBrowsing
			return nil
		}
		return tea.Quit
	}

	// Delegate to input when active.
	if a.state == stateInputting {
		return a.input.Update(msg)
	}

	switch key {
	case "q", "escape":
		if a.state == stateRunning {
			a.runner.stop()
			a.state = stateBrowsing
			a.resizeChildren()
			return nil
		}
		// Don't quit if the list is filtering.
		if g := a.activeGroup(); g != nil && g.list.FilterState() != 0 {
			return a.updateActiveList(msg)
		}
		return tea.Quit
	case "tab":
		a.cycleTab(1)
		return nil
	case "shift+tab":
		a.cycleTab(-1)
		return nil
	case "n":
		if a.state == stateBrowsing {
			return a.startAddSub()
		}
	case "D":
		if a.state == stateBrowsing {
			return a.handleDeleteGroup()
		}
	}

	if a.state == stateRunning {
		var cmd tea.Cmd
		a.logView, cmd = a.logView.Update(msg)
		return cmd
	}

	return a.updateActiveList(msg)
}

func (a *App) updateActiveList(msg tea.Msg) tea.Cmd {
	g := a.activeGroup()
	if g == nil {
		return nil
	}
	var cmd tea.Cmd
	g.list, cmd = g.list.Update(msg)
	return cmd
}

func (a *App) activeGroup() *group {
	if a.tab < 0 || a.tab >= len(a.groups) {
		return nil
	}
	return &a.groups[a.tab]
}

func (a *App) cycleTab(delta int) {
	if len(a.groups) == 0 {
		return
	}
	a.tab = (a.tab + delta + len(a.groups)) % len(a.groups)
}

// --- Connect ---

func (a *App) handleConnect(msg connectMsg) tea.Cmd {
	g := a.activeGroup()
	if g == nil {
		return nil
	}

	outbounds, err := sub.Parse(strings.NewReader(strings.Join(g.data.Nodes, "\n")))
	if err != nil {
		a.err = err
		return nil
	}

	if msg.index >= len(outbounds) {
		return nil
	}

	cfg, err := sub.Config(outbounds, outbounds[msg.index].Tag)
	if err != nil {
		a.err = err
		return nil
	}

	configJSON, err := marshalJSON(cfg)
	if err != nil {
		a.err = err
		return nil
	}

	a.err = nil
	a.state = stateRunning
	a.resizeChildren()
	return a.runner.start(configJSON)
}

func (a *App) handleLogLine(msg logLineMsg) tea.Cmd {
	a.runner.appendLine(string(msg))
	a.logView.SetContent(a.runner.logContent())
	a.logView.GotoBottom()
	return a.runner.waitNext()
}

func (a *App) handleRunnerDone(msg runnerDoneMsg) tea.Cmd {
	a.runner.cmd = nil
	a.state = stateBrowsing
	if msg.err != nil {
		a.err = msg.err
	}
	a.resizeChildren()
	return nil
}

// --- Node editing (local groups only) ---

func (a *App) startAddNode() tea.Cmd {
	g := a.activeGroup()
	if g == nil || g.data.IsSubscription() {
		return nil
	}
	a.state = stateInputting
	a.input.flow = flowAddNode
	return a.input.start(flowAddNode, "Add node URI: ", "", a.width-20)
}

func (a *App) startEditNode(msg editNodeMsg) tea.Cmd {
	g := a.activeGroup()
	if g == nil || g.data.IsSubscription() {
		return nil
	}
	a.state = stateInputting
	return a.input.start(flowEditNode, "Edit node URI: ", msg.nodeURI, a.width-20)
}

func (a *App) handleDeleteNode(msg deleteNodeMsg) tea.Cmd {
	g := a.activeGroup()
	if g == nil || g.data.IsSubscription() || msg.index >= len(g.data.Nodes) {
		return nil
	}
	g.data.Nodes = append(g.data.Nodes[:msg.index], g.data.Nodes[msg.index+1:]...)
	g.syncItems()
	return a.saveGroup(g)
}

// --- Subscription management ---

func (a *App) startAddSub() tea.Cmd {
	a.state = stateInputting
	return a.input.start(flowSubName, "Subscription name: ", "", a.width-25)
}

func (a *App) handleRefresh() tea.Cmd {
	g := a.activeGroup()
	if g == nil || !g.data.IsSubscription() {
		return nil
	}
	return a.fetchSubscription(g.data.Name, g.data.URL)
}

func (a *App) handleDeleteGroup() tea.Cmd {
	g := a.activeGroup()
	if g == nil || g.data.Name == "manual" {
		return nil
	}
	if err := a.store.DeleteGroup(g.data.Name); err != nil {
		a.err = err
		return nil
	}
	a.groups = append(a.groups[:a.tab], a.groups[a.tab+1:]...)
	if a.tab >= len(a.groups) {
		a.tab = max(len(a.groups)-1, 0)
	}
	return nil
}

func (a *App) handleInputResult(msg inputResultMsg) tea.Cmd {
	a.state = stateBrowsing
	value := strings.TrimSpace(msg.value)
	if value == "" {
		return nil
	}

	switch a.input.flow {
	case flowAddNode:
		g := a.activeGroup()
		if g == nil {
			return nil
		}
		g.data.Nodes = append(g.data.Nodes, value)
		g.syncItems()
		g.list.Select(len(g.data.Nodes) - 1)
		return a.saveGroup(g)

	case flowEditNode:
		g := a.activeGroup()
		if g == nil {
			return nil
		}
		idx := g.list.Index()
		if idx < len(g.data.Nodes) {
			g.data.Nodes[idx] = value
			g.syncItems()
		}
		return a.saveGroup(g)

	case flowSubName:
		a.input.subName = value
		a.state = stateInputting
		return a.input.start(flowSubURL, "Subscription URL: ", "", a.width-25)

	case flowSubURL:
		return a.fetchSubscription(a.input.subName, value)
	}

	return nil
}

func (a *App) fetchSubscription(name, subURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(subURL)
		if err != nil {
			return fetchDoneMsg{name: name, err: err}
		}
		defer resp.Body.Close() //nolint:errcheck

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fetchDoneMsg{name: name, err: err}
		}

		decoded := sub.DecodeBody(body)
		lines := splitLines(string(decoded))

		if _, err := sub.Parse(strings.NewReader(string(decoded))); err != nil {
			return fetchDoneMsg{name: name, err: err}
		}

		return fetchDoneMsg{name: name, url: subURL, nodes: lines}
	}
}

func (a *App) handleFetchDone(msg fetchDoneMsg) tea.Cmd {
	if msg.err != nil {
		a.err = msg.err
		return nil
	}

	gData := store.Group{Name: msg.name, Nodes: msg.nodes, URL: msg.url}

	idx := -1
	for i, g := range a.groups {
		if g.data.Name == msg.name {
			idx = i
			break
		}
	}

	if idx >= 0 {
		a.groups[idx].data = gData
		a.groups[idx].syncItems()
	} else {
		a.groups = append(a.groups, newGroup(gData, a.width, a.listHeight()))
		idx = len(a.groups) - 1
	}

	a.tab = idx

	if err := a.store.SaveGroup(gData); err != nil {
		a.err = err
	}

	return nil
}

func (a *App) saveGroup(g *group) tea.Cmd {
	if err := a.store.SaveGroup(g.data); err != nil {
		a.err = err
	}
	return nil
}

// --- View ---

func (a *App) View() tea.View {
	if a.width == 0 || a.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	var sections []string

	if g := a.activeGroup(); g != nil {
		sections = append(sections, g.list.View())
	}

	if a.err != nil {
		sections = append(sections, errStyle.Width(a.width).Render("Error: "+a.err.Error()))
	}

	if a.state == stateInputting {
		sections = append(sections, a.input.View())
	}

	if a.state == stateRunning || len(a.runner.lines) > 0 {
		sections = append(sections, logHeader.Width(a.width).Render("── sing-box ──"))
		sections = append(sections, a.logView.View())
	}

	sections = append(sections, helpBar.Width(a.width).Render(a.helpText()))

	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	view.AltScreen = true
	return view
}


func (a *App) helpText() string {
	if a.state == stateInputting {
		return "enter: confirm · esc: cancel"
	}
	if a.state == stateRunning {
		return "q/esc: stop · tab/shift+tab: switch group"
	}
	g := a.activeGroup()
	parts := []string{"↑↓: navigate", "/: filter", "tab: groups", "enter: connect", "n: new sub"}
	if g != nil && !g.data.IsSubscription() {
		parts = append(parts, "a: add", "e: edit", "d: del")
	}
	if g != nil && g.data.IsSubscription() {
		parts = append(parts, "r: refresh")
	}
	if g != nil && g.data.Name != "manual" {
		parts = append(parts, "D: del group")
	}
	parts = append(parts, "q: quit")
	return strings.Join(parts, " · ")
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
