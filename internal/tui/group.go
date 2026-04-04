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

	l := list.New(items, list.NewDefaultDelegate(), width, height)
	l.Title = titleForGroup(g)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.DisableQuitKeybindings()

	return group{data: g, list: l}
}

func (g *group) setSize(w, h int) {
	g.list.SetSize(w, h)
}

func (g *group) syncItems() tea.Cmd {
	g.list.Title = titleForGroup(g.data)

	items := make([]list.Item, len(g.data.Nodes))
	for i, n := range g.data.Nodes {
		items[i] = nodeItem{uri: n}
	}
	return g.list.SetItems(items)
}

func titleForGroup(g store.Group) string {
	if g.IsRemote() {
		return g.Name + " (remote)"
	}
	return g.Name
}
