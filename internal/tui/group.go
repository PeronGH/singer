package tui

import (
	"net/url"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/PeronGH/singer/internal/store"
)

// nodeItem implements list.DefaultItem for display in the list.
type nodeItem struct {
	uri string
}

func (n nodeItem) Title() string       { return extractTag(n.uri) }
func (n nodeItem) Description() string { return extractDesc(n.uri) }
func (n nodeItem) FilterValue() string { return extractTag(n.uri) }

func extractTag(uri string) string {
	if idx := strings.LastIndex(uri, "#"); idx >= 0 && idx < len(uri)-1 {
		return uri[idx+1:]
	}
	return uri
}

func extractDesc(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	host := u.Hostname()
	if port := u.Port(); port != "" {
		host += ":" + port
	}
	return u.Scheme + " · " + host
}

// group wraps a store.Group and a list.Model for one tab.
type group struct {
	data store.Group
	list list.Model
}

func newGroup(g store.Group, width, height int) group {
	items := make([]list.Item, len(g.Nodes))
	for i, n := range g.Nodes {
		items[i] = nodeItem{uri: n}
	}

	delegate := list.NewDefaultDelegate()
	delegate.UpdateFunc = delegateUpdate

	l := list.New(items, delegate, width, height)
	l.Title = g.Name
	if g.IsSubscription() {
		l.Title += " ↻"
	}
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()

	return group{data: g, list: l}
}

func (g *group) setSize(w, h int) {
	g.list.SetSize(w, h)
}

func (g *group) syncItems() {
	items := make([]list.Item, len(g.data.Nodes))
	for i, n := range g.data.Nodes {
		items[i] = nodeItem{uri: n}
	}
	g.list.SetItems(items)
}

// delegateUpdate handles per-item key presses, returning action messages.
func delegateUpdate(msg tea.Msg, m *list.Model) tea.Cmd {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}

	switch km.String() {
	case "enter":
		if item := m.SelectedItem(); item != nil {
			uri := item.(nodeItem).uri
			return func() tea.Msg { return connectMsg{nodeURI: uri} }
		}
	case "a":
		return func() tea.Msg { return addNodeMsg{} }
	case "e":
		if item := m.SelectedItem(); item != nil {
			uri := item.(nodeItem).uri
			return func() tea.Msg { return editNodeMsg{nodeURI: uri} }
		}
	case "d":
		idx := m.Index()
		return func() tea.Msg { return deleteNodeMsg{index: idx} }
	case "r":
		return func() tea.Msg { return refreshMsg{} }
	}

	return nil
}

