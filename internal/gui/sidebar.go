package gui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *App) sidebarView(c *ui.Context) {
	t := c.Theme()
	var refreshTarget string
	var deleteTarget string

	ui.Column(c).Width(220).Shrink(0).FillHeight().Background(t.Surface).Children(func() {
		ui.Row(c).Padding(14, 14, 10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Singer").FontSize(16).Bold().Grow(1)
			if ui.Button(c, "+").Label("Add remote").Tooltip("Add remote group").Padding(4, 9).Clicked() {
				a.openAddRemoteModal()
			}
		})

		ui.Sidebar(c, &a.selectedNav, func() {
			ui.SidebarSection(c, "Groups", nil, func() {
				for _, g := range a.groups {
					id := navGroupPrefix + g.Name
					item := ui.SidebarItem(c.Key(id), id, nil, g.Name).Children(func() {
						if a.fetching[g.Name] {
							ui.Spinner(c).Label("Refreshing " + g.Name)
						} else {
							ui.Badge(c, fmt.Sprint(len(g.Nodes)))
						}
					})
					if g.IsRemote() {
						item.ContextMenu(func(m *ui.Menu) {
							if m.Item("Refresh").Disabled(a.fetching[g.Name]).Chosen() {
								refreshTarget = g.Name
							}
							m.Separator()
							if m.Item("Delete…").Chosen() {
								deleteTarget = g.Name
							}
						})
					}
				}
			})

			ui.SidebarSection(c, "Runtime", nil, func() {
				ui.SidebarItem(c.Key(navLogs), navLogs, nil, "Logs").Children(func() {
					if a.runner.active {
						ui.Badge(c, "Running").Background(t.Success).TextColor(t.AccentText)
					}
				})
			})
		}).Grow(1).Label("Navigation")

		ui.Divider(c)
		a.statusFooterView(c)
	})

	if refreshTarget != "" {
		if g := a.findGroup(refreshTarget); g != nil {
			a.startFetchRemote(g.Name, g.URL)
		}
	}
	if deleteTarget != "" {
		a.confirmDeleteRemoteName = deleteTarget
		a.confirmDeleteRemoteOpen = true
	}
}

func (a *App) statusFooterView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Padding(12, 14).Gap(8).Shrink(0).Children(func() {
		if a.runner.active {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.Success).Shrink(0)
				ui.Text(c, "Connected").FontSize(12).Bold().TextColor(t.Success)
			})
			ui.Text(c, a.runner.activeTag).FontSize(13).Bold().SingleLine()
			ui.Text(c, a.runner.activeGroup).FontSize(12).TextColor(t.TextMuted).SingleLine()
			ui.Row(c).Gap(6).Children(func() {
				if ui.Button(c, "Stop").Label("Stop sing-box").Grow(1).Clicked() {
					a.stopRunner()
				}
				if ui.Button(c, "Logs").Label("Open logs").Clicked() {
					a.selectedNav = navLogs
				}
			})
			return
		}

		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(t.TextMuted.Alpha(0.5)).Shrink(0)
			ui.Text(c, "Disconnected").FontSize(12).TextColor(t.TextMuted)
		})
	})
}
