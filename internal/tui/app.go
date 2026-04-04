package tui

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/PeronGH/singer/internal/store"
	"github.com/PeronGH/singer/sub"
)

type appMode int

const (
	modeBrowsing appMode = iota
	modePrompt
)

type activeTabKind int

const (
	tabGroups activeTabKind = iota
	tabLogs
)

const (
	tabBarH  = 1
	helpBarH = 1
)

type App struct {
	store  *store.Store
	client *http.Client

	browser browserModel
	prompt  promptModel
	runner  runnerModel

	activeTab        activeTabKind
	helpVisible      bool
	mode             appMode
	width, height    int
	editingNodeIndex int
	pendingRun       []byte
	err              error
}

func NewApp(s *store.Store) (*App, error) {
	groups, err := s.LoadGroups()
	if err != nil {
		return nil, err
	}

	return &App{
		store:            s,
		client:           &http.Client{Timeout: 15 * time.Second},
		browser:          newBrowser(groups, 0, 0),
		prompt:           newPrompt(0),
		runner:           newRunner(),
		activeTab:        tabGroups,
		editingNodeIndex: -1,
	}, nil
}

func (a *App) Init() tea.Cmd {
	return nil
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.resizeChildren()
		return a, nil
	case tea.KeyPressMsg:
		if matchCtrlLetter(msg, 'c') {
			if a.runner.active {
				a.runner.stop()
				return a, nil
			}
			return a, tea.Quit
		}
		if matchPrintable(msg, '?') {
			a.helpVisible = !a.helpVisible
			return a, nil
		}
		if a.helpVisible && key.Matches(msg, stopKey) {
			a.helpVisible = false
			return a, nil
		}
	case logLineMsg:
		return a, a.runner.appendLine(string(msg))
	case runnerDoneMsg:
		return a, a.handleRunnerDone(msg)
	case fetchDoneMsg:
		return a, a.handleFetchDone(msg)
	}

	if a.helpVisible {
		return a, nil
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok && a.mode != modePrompt && !a.browserFiltering() {
		switch {
		case key.Matches(msg, nextTabKey):
			a.cycleTabs(1)
			return a, nil
		case key.Matches(msg, prevTabKey):
			a.cycleTabs(-1)
			return a, nil
		}
	}

	switch a.mode {
	case modePrompt:
		action, cmd := a.prompt.update(msg)
		return a, tea.Batch(cmd, a.handlePromptAction(action))
	default:
		if a.activeTab == tabLogs {
			return a, a.updateLogsTab(msg)
		}
		action, cmd := a.browser.update(msg)
		return a, tea.Batch(cmd, a.handleBrowserAction(action))
	}
}

func (a *App) View() tea.View {
	if a.width == 0 || a.height == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}

	if a.helpVisible {
		sections := []string{
			a.helpScreenView(),
			helpBar.Width(a.width).Render(a.helpFooterText()),
		}
		view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
		view.AltScreen = true
		return view
	}

	sections := []string{
		a.tabBarView(),
		a.activeContentView(),
	}

	if a.err != nil {
		sections = append(sections, errStyle.Width(a.width).Render("Error: "+a.err.Error()))
	}

	if a.mode == modePrompt {
		sections = append(sections, a.prompt.view())
	}

	sections = append(sections, helpBar.Width(a.width).Render(a.helpText()))

	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, sections...))
	view.AltScreen = true
	return view
}

func (a *App) resizeChildren() {
	a.browser.setSize(a.width, a.contentHeight())
	a.prompt.setWidth(a.width)
	a.runner.setSize(a.width, a.contentHeight())
}

func (a *App) contentHeight() int {
	h := a.height - tabBarH - helpBarH
	if a.err != nil {
		h--
	}
	if a.mode == modePrompt {
		h--
	}
	return max(h, 1)
}

func (a *App) helpText() string {
	switch a.mode {
	case modePrompt:
		return "enter: confirm · esc: cancel · ?: help"
	default:
		return "tab: next tab · q: quit · ?: help"
	}
}

func (a *App) helpFooterText() string {
	return "?: close help · esc: close help · ctrl+c: quit"
}

func (a *App) helpScreenView() string {
	lines := []string{
		helpTitle.Render("Singer Help"),
		"",
		helpSection.Render("Global"),
		"ctrl+c            Quit the app, or stop sing-box while it is running",
		"?                 Open or close this help screen",
		"tab/shift+tab     Switch between local, remotes, and logs",
		"",
		helpSection.Render("Group Tabs"),
		"↑/↓               Move through nodes",
		"/                 Start filtering",
		"enter             Connect selected node",
		"n                 Add a new remote group",
		"R                 Refresh all remote groups",
		"q                 Quit",
		"",
		helpSection.Render("Logs Tab"),
		"↑/↓               Scroll logs line by line",
		"pgup/pgdn         Scroll logs by page",
		"home/end          Jump to top or bottom",
		"esc               Stop sing-box while it is running",
	}

	if a.mode == modePrompt {
		lines = append(lines,
			"",
			helpSection.Render("Prompt"),
			"enter             Confirm the current input",
			"esc               Cancel and return to browsing",
		)
	} else if g := a.browser.activeGroupData(); g != nil {
		switch {
		case g.IsLocal():
			lines = append(lines,
				"",
				helpSection.Render("Local Group"),
				"a                 Add a node",
				"e                 Edit the selected node",
				"d                 Delete the selected node",
			)
		case g.IsRemote():
			lines = append(lines,
				"",
				helpSection.Render("Remote Group"),
				"r                 Refresh the selected remote group",
				"D                 Delete the selected remote group",
			)
		}
	}

	return helpScreen.
		Width(a.width).
		Height(max(a.height-helpBarH, 1)).
		Render(strings.Join(lines, "\n"))
}

func (a *App) handleBrowserAction(action browserAction) tea.Cmd {
	switch action.Kind {
	case browserActionNone:
		return nil
	case browserActionQuit:
		return tea.Quit
	case browserActionConnect:
		return a.handleConnect(action.GroupName, action.NodeIndex)
	case browserActionAddRemote:
		a.err = nil
		a.mode = modePrompt
		a.prompt.remoteName = ""
		return a.prompt.start(promptFlowRemoteName, "Remote name (esc cancels): ", "")
	case browserActionRefreshRemote:
		g := a.findGroup(action.GroupName)
		if g == nil || !g.data.IsRemote() {
			return nil
		}
		a.err = nil
		return fetchRemoteCmd(a.client, g.data.Name, g.data.URL)
	case browserActionRefreshAll:
		remotes := a.browser.remoteGroups()
		if len(remotes) == 0 {
			return nil
		}

		a.err = nil
		cmds := make([]tea.Cmd, 0, len(remotes))
		for _, g := range remotes {
			cmds = append(cmds, fetchRemoteCmd(a.client, g.Name, g.URL))
		}
		return tea.Batch(cmds...)
	case browserActionDeleteRemote:
		if err := a.store.DeleteRemote(action.GroupName); err != nil {
			a.err = err
			return nil
		}
		a.err = nil
		a.browser.removeRemote(action.GroupName)
		return a.browser.activeStatus("deleted remote " + action.GroupName)
	case browserActionAddLocalNode:
		a.err = nil
		a.mode = modePrompt
		a.editingNodeIndex = -1
		return a.prompt.start(promptFlowAddLocalNode, "Local node URI (esc cancels): ", "")
	case browserActionEditLocalNode:
		a.err = nil
		a.mode = modePrompt
		a.editingNodeIndex = action.NodeIndex
		return a.prompt.start(promptFlowEditLocalNode, "Edit local node URI (esc cancels): ", action.NodeURI)
	case browserActionDeleteLocalNode:
		return a.deleteLocalNode(action.NodeIndex)
	default:
		return nil
	}
}

func (a *App) handlePromptAction(action promptAction) tea.Cmd {
	switch action.Kind {
	case promptActionNone:
		return nil
	case promptActionCancel:
		a.mode = modeBrowsing
		a.prompt.reset()
		a.editingNodeIndex = -1
		return nil
	case promptActionConfirm:
		value := strings.TrimSpace(action.Value)
		switch a.prompt.flow {
		case promptFlowRemoteName:
			if value == "" {
				a.mode = modeBrowsing
				a.prompt.reset()
				return nil
			}
			if err := store.ValidateRemoteName(value); err != nil {
				a.err = err
				return a.prompt.start(promptFlowRemoteName, "Remote name (esc cancels): ", value)
			}

			a.err = nil
			a.mode = modePrompt
			a.prompt.remoteName = value
			return a.prompt.start(promptFlowRemoteURL, "Remote URL (esc cancels): ", "")
		case promptFlowRemoteURL:
			name := a.prompt.remoteName
			a.mode = modeBrowsing
			a.prompt.reset()
			if value == "" {
				return nil
			}
			a.err = nil
			return fetchRemoteCmd(a.client, name, value)
		case promptFlowAddLocalNode:
			a.mode = modeBrowsing
			a.prompt.reset()
			if value == "" {
				return nil
			}

			local := a.localGroup()
			if local == nil {
				a.err = fmt.Errorf("local group not loaded")
				return nil
			}

			nodes := append(append([]string(nil), local.data.Nodes...), value)
			return a.saveLocalNodes(nodes, "added local node")
		case promptFlowEditLocalNode:
			a.mode = modeBrowsing
			a.prompt.reset()
			if value == "" {
				a.editingNodeIndex = -1
				return nil
			}
			return a.replaceLocalNode(a.editingNodeIndex, value)
		default:
			return nil
		}
	default:
		return nil
	}
}

func (a *App) handleConnect(groupName string, nodeIndex int) tea.Cmd {
	g := a.findGroup(groupName)
	if g == nil {
		return nil
	}

	_, outbounds, err := sub.ParseBody([]byte(strings.Join(g.data.Nodes, "\n")))
	if err != nil {
		a.err = err
		return nil
	}
	if nodeIndex < 0 || nodeIndex >= len(outbounds) {
		return nil
	}

	cfg, err := sub.Config(outbounds, outbounds[nodeIndex].Tag)
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
	a.activeTab = tabLogs
	if a.runner.active {
		a.pendingRun = configJSON
		if !a.runner.stopping {
			a.runner.stop()
		}
		return nil
	}
	return a.runner.start(configJSON)
}

func (a *App) handleFetchDone(msg fetchDoneMsg) tea.Cmd {
	if msg.err != nil {
		a.err = msg.err
		return nil
	}

	if err := a.store.UpsertRemote(msg.group.Name, msg.group.URL, msg.group.Nodes); err != nil {
		a.err = err
		return nil
	}

	a.err = nil
	cmd := a.browser.upsertGroup(msg.group)
	return tea.Batch(cmd, a.browser.activeStatus("updated "+msg.group.Name))
}

func (a *App) handleRunnerDone(msg runnerDoneMsg) tea.Cmd {
	if err := a.runner.finish(msg); err != nil {
		a.err = err
	} else {
		a.err = nil
	}
	a.resizeChildren()

	if a.pendingRun != nil {
		configJSON := a.pendingRun
		a.pendingRun = nil
		a.err = nil
		a.activeTab = tabLogs
		return a.runner.start(configJSON)
	}

	return nil
}

func (a *App) deleteLocalNode(index int) tea.Cmd {
	local := a.localGroup()
	if local == nil {
		a.err = fmt.Errorf("local group not loaded")
		return nil
	}
	if index < 0 || index >= len(local.data.Nodes) {
		return nil
	}

	nodes := append(append([]string(nil), local.data.Nodes[:index]...), local.data.Nodes[index+1:]...)
	return a.saveLocalNodes(nodes, "deleted local node")
}

func (a *App) replaceLocalNode(index int, value string) tea.Cmd {
	defer func() {
		a.editingNodeIndex = -1
	}()

	local := a.localGroup()
	if local == nil {
		a.err = fmt.Errorf("local group not loaded")
		return nil
	}
	if index < 0 || index >= len(local.data.Nodes) {
		return nil
	}

	nodes := append([]string(nil), local.data.Nodes...)
	nodes[index] = value
	return a.saveLocalNodes(nodes, "updated local node")
}

func (a *App) saveLocalNodes(nodes []string, status string) tea.Cmd {
	if err := a.store.SaveLocal(nodes); err != nil {
		a.err = err
		return nil
	}

	a.err = nil
	cmd := a.browser.updateLocalNodes(nodes)
	return tea.Batch(cmd, a.browser.activeStatus(status))
}

func (a *App) localGroup() *group {
	for i := range a.browser.groups {
		if a.browser.groups[i].data.IsLocal() {
			return &a.browser.groups[i]
		}
	}
	return nil
}

func (a *App) findGroup(name string) *group {
	for i := range a.browser.groups {
		if a.browser.groups[i].data.Name == name {
			return &a.browser.groups[i]
		}
	}
	return nil
}

func (a *App) updateLogsTab(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case matchLetter(msg, 'q'):
			return tea.Quit
		case key.Matches(msg, stopKey) && a.runner.active:
			a.runner.stop()
			return nil
		}
	}
	return a.runner.update(msg)
}

func (a *App) browserFiltering() bool {
	if a.activeTab != tabGroups {
		return false
	}
	if g := a.browser.activeGroup(); g != nil {
		return g.list.SettingFilter()
	}
	return false
}

func (a *App) cycleTabs(delta int) {
	groupCount := len(a.browser.groups)
	if groupCount == 0 {
		a.activeTab = tabLogs
		return
	}

	switch a.activeTab {
	case tabLogs:
		a.activeTab = tabGroups
		if delta > 0 {
			a.browser.tab = 0
		} else {
			a.browser.tab = groupCount - 1
		}
	default:
		next := a.browser.tab + delta
		if next < 0 || next >= groupCount {
			a.activeTab = tabLogs
			return
		}
		a.browser.tab = next
	}
}

func (a *App) tabBarView() string {
	tabs := make([]string, 0, len(a.browser.groups)+1)
	for i, g := range a.browser.groups {
		style := inactiveTabStyle
		if a.activeTab == tabGroups && i == a.browser.tab {
			style = activeTabStyle
		}
		tabs = append(tabs, style.Render(g.data.Name))
	}

	logsLabel := "logs"
	if a.runner.active {
		logsLabel = "logs*"
	}
	style := inactiveTabStyle
	if a.activeTab == tabLogs {
		style = activeTabStyle
	}
	tabs = append(tabs, style.Render(logsLabel))

	return tabBar.Width(a.width).Render(lipgloss.JoinHorizontal(lipgloss.Left, tabs...))
}

func (a *App) activeContentView() string {
	if a.activeTab == tabLogs {
		return a.runner.view()
	}
	if g := a.browser.activeGroup(); g != nil {
		return g.list.View()
	}
	return ""
}

func fetchRemoteCmd(client *http.Client, name, remoteURL string) tea.Cmd {
	return func() tea.Msg {
		remoteURL = strings.TrimSpace(remoteURL)
		req, err := http.NewRequest(http.MethodGet, remoteURL, nil)
		if err != nil {
			return fetchDoneMsg{err: fmt.Errorf("fetch %q: %w", name, err)}
		}

		resp, err := client.Do(req)
		if err != nil {
			return fetchDoneMsg{err: fmt.Errorf("fetch %q: %w", name, err)}
		}
		defer resp.Body.Close() //nolint:errcheck

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fetchDoneMsg{err: fmt.Errorf("fetch %q: unexpected status %s", name, resp.Status)}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fetchDoneMsg{err: fmt.Errorf("fetch %q: %w", name, err)}
		}

		lines, _, err := sub.ParseBody(body)
		if err != nil {
			return fetchDoneMsg{err: fmt.Errorf("fetch %q: %w", name, err)}
		}

		return fetchDoneMsg{
			group: store.Group{
				Name:  name,
				Kind:  store.GroupKindRemote,
				Nodes: lines,
				URL:   remoteURL,
			},
		}
	}
}
