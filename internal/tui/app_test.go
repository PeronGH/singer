package tui

import (
	"errors"
	"testing"

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

func TestPromptPasteUsesTextInputBehavior(t *testing.T) {
	prompt := newPrompt(40)
	if cmd := prompt.start(promptFlowAddLocalNode, "Local node URI: ", "ab"); cmd == nil {
		t.Fatal("expected focus command")
	}
	prompt.input.SetCursor(1)

	action, _ := prompt.update(tea.PasteMsg{Content: "XYZ"})
	if action.Kind != promptActionNone {
		t.Fatalf("action kind = %v, want none", action.Kind)
	}
	if got := prompt.input.Value(); got != "aXYZb" {
		t.Fatalf("value = %q, want %q", got, "aXYZb")
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

func keyPress(k tea.Key) tea.KeyPressMsg {
	return tea.KeyPressMsg(k)
}
