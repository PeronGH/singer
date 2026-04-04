package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/PeronGH/singer/internal/store"
)

type browserActionKind int

const (
	browserActionNone browserActionKind = iota
	browserActionConnect
	browserActionAddRemote
	browserActionRefreshRemote
	browserActionRefreshAll
	browserActionDeleteRemote
	browserActionAddLocalNode
	browserActionEditLocalNode
	browserActionDeleteLocalNode
	browserActionQuit
)

type browserAction struct {
	Kind      browserActionKind
	GroupName string
	NodeIndex int
	NodeURI   string
}

type browserModel struct {
	groups []group
	tab    int
	width  int
	height int
}

var (
	connectKey      = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "connect"))
	refreshAllKey   = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh all"))
	addLocalKey     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add node"))
	deleteRemoteKey = key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "delete remote"),
	)
)

func newBrowser(groups []store.Group, width, height int) browserModel {
	b := browserModel{
		groups: make([]group, len(groups)),
		width:  width,
		height: height,
	}
	for i, g := range groups {
		b.groups[i] = newGroup(g, width, height)
	}
	return b
}

func (b *browserModel) setSize(width, height int) {
	b.width = width
	b.height = height
	for i := range b.groups {
		b.groups[i].setSize(width, height)
	}
}

func (b *browserModel) activeGroup() *group {
	if b.tab < 0 || b.tab >= len(b.groups) {
		return nil
	}
	return &b.groups[b.tab]
}

func (b *browserModel) activeGroupData() *store.Group {
	g := b.activeGroup()
	if g == nil {
		return nil
	}
	return &g.data
}

func (b *browserModel) activeGroupName() string {
	if g := b.activeGroupData(); g != nil {
		return g.Name
	}
	return ""
}

func (b *browserModel) activeStatus(message string) tea.Cmd {
	g := b.activeGroup()
	if g == nil {
		return nil
	}
	return g.list.NewStatusMessage(message)
}

func (b *browserModel) remoteGroups() []store.Group {
	remotes := make([]store.Group, 0, len(b.groups))
	for _, g := range b.groups {
		if g.data.IsRemote() {
			remotes = append(remotes, g.data)
		}
	}
	return remotes
}

func (b *browserModel) update(msg tea.Msg) (browserAction, tea.Cmd) {
	g := b.activeGroup()
	if g == nil {
		return browserAction{}, nil
	}

	if g.list.SettingFilter() {
		var cmd tea.Cmd
		g.list, cmd = g.list.Update(msg)
		return browserAction{}, cmd
	}

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, connectKey):
			idx, ok := b.selectedNodeIndex(g)
			if !ok {
				return browserAction{}, nil
			}
			return browserAction{
				Kind:      browserActionConnect,
				GroupName: g.data.Name,
				NodeIndex: idx,
			}, nil
		case matchLetter(msg, 'n'):
			return browserAction{Kind: browserActionAddRemote}, nil
		case key.Matches(msg, refreshAllKey):
			return browserAction{Kind: browserActionRefreshAll}, nil
		case matchLetter(msg, 'q'):
			return browserAction{Kind: browserActionQuit}, nil
		case key.Matches(msg, addLocalKey) && g.data.IsLocal():
			return browserAction{Kind: browserActionAddLocalNode}, nil
		case matchLetter(msg, 'e') && g.data.IsLocal():
			idx, ok := b.selectedNodeIndex(g)
			if !ok {
				return browserAction{}, nil
			}
			return browserAction{
				Kind:      browserActionEditLocalNode,
				GroupName: g.data.Name,
				NodeIndex: idx,
				NodeURI:   g.data.Nodes[idx],
			}, nil
		case matchLetter(msg, 'd') && g.data.IsLocal():
			idx, ok := b.selectedNodeIndex(g)
			if !ok {
				return browserAction{}, nil
			}
			return browserAction{
				Kind:      browserActionDeleteLocalNode,
				GroupName: g.data.Name,
				NodeIndex: idx,
			}, nil
		case matchLetter(msg, 'r') && g.data.IsRemote():
			return browserAction{
				Kind:      browserActionRefreshRemote,
				GroupName: g.data.Name,
			}, nil
		case key.Matches(msg, deleteRemoteKey) && g.data.IsRemote():
			return browserAction{
				Kind:      browserActionDeleteRemote,
				GroupName: g.data.Name,
			}, nil
		}
	}

	var cmd tea.Cmd
	g.list, cmd = g.list.Update(msg)
	return browserAction{}, cmd
}

func (b *browserModel) upsertGroup(data store.Group) tea.Cmd {
	targetName := data.Name
	found := false
	var cmd tea.Cmd

	for i := range b.groups {
		if b.groups[i].data.Name != data.Name {
			continue
		}

		found = true
		b.groups[i].data = data
		cmd = b.groups[i].syncItems()
		break
	}

	if !found {
		b.groups = append(b.groups, newGroup(data, b.width, b.height))
	}

	b.sortGroups()
	for i := range b.groups {
		if b.groups[i].data.Name == targetName {
			b.tab = i
			break
		}
	}
	return cmd
}

func (b *browserModel) removeRemote(name string) {
	for i := range b.groups {
		if b.groups[i].data.Name != name {
			continue
		}
		b.groups = slices.Delete(b.groups, i, i+1)
		if len(b.groups) == 0 {
			b.tab = 0
			return
		}
		if b.tab >= len(b.groups) {
			b.tab = len(b.groups) - 1
		}
		return
	}
}

func (b *browserModel) updateLocalNodes(nodes []string) tea.Cmd {
	for i := range b.groups {
		if !b.groups[i].data.IsLocal() {
			continue
		}

		b.groups[i].data.Nodes = append([]string(nil), nodes...)
		cmd := b.groups[i].syncItems()
		b.tab = i
		return cmd
	}
	return nil
}

func (b *browserModel) sortGroups() {
	if len(b.groups) <= 1 {
		return
	}

	activeName := b.activeGroupName()
	slices.SortStableFunc(b.groups, func(a, c group) int {
		switch {
		case a.data.IsLocal() && !c.data.IsLocal():
			return -1
		case !a.data.IsLocal() && c.data.IsLocal():
			return 1
		default:
			return strings.Compare(a.data.Name, c.data.Name)
		}
	})

	for i := range b.groups {
		if b.groups[i].data.Name == activeName {
			b.tab = i
		}
	}
}

func (b *browserModel) selectedNodeIndex(g *group) (int, bool) {
	if g == nil || g.list.SelectedItem() == nil {
		return 0, false
	}
	idx := g.list.GlobalIndex()
	if idx < 0 || idx >= len(g.data.Nodes) {
		return 0, false
	}
	return idx, true
}
