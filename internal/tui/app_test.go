package tui

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/PeronGH/singer/internal/store"
)

func TestBrowserConnectUsesGlobalIndexWhenFiltered(t *testing.T) {
	browser := newBrowser([]store.Group{{
		Name: "local",
		Kind: store.GroupKindLocal,
		Nodes: []string{
			"socks://10.0.0.1:1080#alpha",
			"socks://10.0.0.2:1080#bravo",
			"socks://10.0.0.3:1080#charlie",
		},
	}}, 80, 20)

	browser.activeGroup().list.SetFilterText("bravo")

	action, _ := browser.update(keyPress(tea.Key{Code: tea.KeyEnter}))
	if action.Kind != browserActionConnect {
		t.Fatalf("action kind = %v, want connect", action.Kind)
	}
	if action.NodeIndex != 1 {
		t.Fatalf("node index = %d, want 1", action.NodeIndex)
	}
}

func TestBrowserDoesNotFireAppShortcutsWhileFiltering(t *testing.T) {
	browser := newBrowser([]store.Group{{
		Name:  "local",
		Kind:  store.GroupKindLocal,
		Nodes: []string{"socks://10.0.0.1:1080#alpha"},
	}}, 80, 20)

	g := browser.activeGroup()
	g.list.SetFilterState(list.Filtering)
	g.list.FilterInput.Focus()

	action, _ := browser.update(keyPress(tea.Key{Text: "n", Code: 'n'}))
	if action.Kind != browserActionNone {
		t.Fatalf("action kind = %v, want none while filtering", action.Kind)
	}
	if got := g.list.FilterValue(); got != "n" {
		t.Fatalf("filter value = %q, want %q", got, "n")
	}
}

func TestRunnerFinishSuppressesIntentionalStopErrors(t *testing.T) {
	runner := newRunner()
	runner.active = true
	runner.stopping = true

	if err := runner.finish(runnerDoneMsg{err: errors.New("killed")}); err != nil {
		t.Fatalf("finish error = %v, want nil", err)
	}
	if runner.active {
		t.Fatal("runner should no longer be active")
	}
	if runner.stopping {
		t.Fatal("runner stopping flag should be cleared")
	}
}

func TestAppCycleTabsIncludesLogs(t *testing.T) {
	app := testApp()
	app.browser.upsertGroup(store.Group{Name: "remote", Kind: store.GroupKindRemote})
	app.browser.tab = 0

	app.cycleTabs(1)
	if app.activeTab != tabGroups || app.browser.tab != 1 {
		t.Fatalf("expected remote tab, got active=%v tab=%d", app.activeTab, app.browser.tab)
	}

	app.cycleTabs(1)
	if app.activeTab != tabLogs {
		t.Fatalf("expected logs tab, got %v", app.activeTab)
	}

	app.cycleTabs(1)
	if app.activeTab != tabGroups || app.browser.tab != 0 {
		t.Fatalf("expected local tab after logs, got active=%v tab=%d", app.activeTab, app.browser.tab)
	}
}

func TestLogsTabOwnsNavigationInput(t *testing.T) {
	app := testApp()
	app.browser.activeGroup().data.Nodes = []string{
		"socks://10.0.0.1:1080#a",
		"socks://10.0.0.2:1080#b",
	}
	if cmd := app.browser.activeGroup().syncItems(); cmd != nil {
		_ = cmd()
	}
	app.browser.setSize(80, 10)
	app.runner.setSize(80, 3)
	for i := 0; i < 10; i++ {
		app.runner.appendRawLine("line")
	}

	app.activeTab = tabLogs
	if _, _ = app.Update(keyPress(tea.Key{Code: tea.KeyDown})); app.browser.activeGroup().list.Index() != 0 {
		t.Fatal("group selection should not move while logs tab is active")
	}
}

func TestHandleConnectQueuesRestartWhileRunnerActive(t *testing.T) {
	app := testApp()
	app.browser.activeGroup().data.Nodes = []string{"socks://10.0.0.1:1080#alpha"}
	app.runner.active = true
	app.runner.stopping = false

	cmd := app.handleConnect("local", 0)
	if cmd != nil {
		t.Fatal("expected restart to be queued, not started immediately")
	}
	if app.pendingRun == nil {
		t.Fatal("expected pending restart config")
	}
	if app.activeTab != tabLogs {
		t.Fatal("expected logs tab to stay active during restart")
	}
}

func testApp() *App {
	groups := []store.Group{{
		Name: "local",
		Kind: store.GroupKindLocal,
		Nodes: []string{
			"socks://10.0.0.1:1080#alpha",
		},
	}}

	app := &App{
		client:    &http.Client{Timeout: time.Second},
		browser:   newBrowser(groups, 80, 20),
		prompt:    newPrompt(80),
		runner:    newRunner(),
		activeTab: tabGroups,
		mode:      modeBrowsing,
		width:     80,
		height:    24,
	}
	app.resizeChildren()
	return app
}

func keyPress(k tea.Key) tea.KeyPressMsg {
	return tea.KeyPressMsg(k)
}
