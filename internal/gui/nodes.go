package gui

import (
	"fmt"

	"github.com/PeronGH/singer/internal/store"
	"github.com/egoist/mygo/ui"
)

func (a *App) groupView(c *ui.Context, g *store.Group) {
	t := c.Theme()
	groupName := g.Name
	isLocal := g.IsLocal()
	isRemote := g.IsRemote()
	remoteURL := g.URL
	filtered := filterNodes(g.Nodes, a.query)

	var connectIdx = -1
	var editIdx = -1
	var editURI string
	var deleteIdx = -1
	var stopRequested bool

	ui.Column(c).Fill().Padding(16, 20).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, groupName).FontSize(18).Bold().SingleLine()
				if isRemote && remoteURL != "" {
					ui.Text(c, remoteURL).FontSize(12).TextColor(t.TextMuted).SingleLine()
				}
			})

			if isLocal {
				if ui.PrimaryButton(c, "Add Node").Clicked() {
					a.openAddLocalNodeModal()
				}
			}
			if isRemote {
				refreshLabel := "Refresh"
				if a.fetching[groupName] {
					refreshLabel = "Refreshing…"
				}
				if ui.Button(c, refreshLabel).Disabled(a.fetching[groupName]).Clicked() {
					a.startFetchRemote(groupName, remoteURL)
				}
				if len(a.remoteGroups()) > 1 {
					if ui.Button(c, "Refresh All").Clicked() {
						a.refreshAllRemotes()
					}
				}
				if ui.Button(c, "Delete").Label("Delete remote " + groupName).Clicked() {
					a.confirmDeleteRemoteName = groupName
					a.confirmDeleteRemoteOpen = true
				}
			}
		})

		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.SearchField(c.Key("filter-"+groupName), &a.query).
				Placeholder("Filter nodes…").
				Label("Filter nodes").
				Bind(&a.searchHandle).
				Grow(1).
				Changed() {
				a.selectedNode = -1
			}
			if a.query != "" {
				ui.Textf(c, "%d of %d", len(filtered), len(g.Nodes)).FontSize(12).TextColor(t.TextMuted)
			}
		})

		a.errorBannerView(c)

		a.nodeList.Selected = &a.selectedNode
		a.nodeList.Key = func(i int) any { return filtered[i].Index }
		a.nodeList.Label = func(i int) string { return filtered[i].Tag }

		listEl := ui.List(c.Key("nodes-"+groupName), &a.nodeList, len(filtered)).
			Grow(1).
			KeepScrollbars(true).
			Label(groupName + " nodes").
			Rows(func(row ui.ListRow) {
				entry := filtered[row.Index]
				active := a.runner.isNodeActive(groupName, entry.Index)
				selected := row.Selected()

				textColor := t.Text
				mutedColor := t.TextMuted
				if selected && row.ListFocused() {
					textColor = t.AccentText
					mutedColor = t.AccentText.Alpha(0.8)
				}

				rowBox := ui.Row(row.Context).
					Padding(10, 12).
					Gap(10).
					AlignItems(ui.Center).
					FillWidth()

				if rowBox.RowDoubleClicked() {
					connectIdx = entry.Index
				}

				rowBox.ContextMenu(func(m *ui.Menu) {
					if active {
						if m.Item("Stop").Chosen() {
							stopRequested = true
						}
					} else {
						if m.Item("Connect").Chosen() {
							connectIdx = entry.Index
						}
					}
					if m.Item("Copy URI").Chosen() {
						row.Context.Services().WriteClipboard(entry.URI)
						a.queueToast("Copied URI")
					}
					if isLocal {
						m.Separator()
						if m.Item("Edit…").Chosen() {
							editIdx = entry.Index
							editURI = entry.URI
						}
						if m.Item("Delete").Chosen() {
							deleteIdx = entry.Index
						}
					}
				})

				rowBox.Children(func() {
					ui.Column(row.Context).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(row.Context).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(row.Context, entry.Tag).Bold().TextColor(textColor).SingleLine()
							if active {
								ui.Badge(row.Context, "Active").Background(t.Success).TextColor(t.AccentText)
							}
						})
						ui.Text(row.Context, entry.Desc).FontSize(12).TextColor(mutedColor).SingleLine()
					})

					if active {
						ui.Button(row.Context, "Stop").
							Label("Stop " + entry.Tag).
							OnClick(func() { a.stopRunner() })
					} else {
						idx := entry.Index
						ui.Button(row.Context, "Connect").
							Label("Connect " + entry.Tag).
							OnClick(func() { a.connectNode(groupName, idx) })
					}

					if isLocal {
						idx := entry.Index
						uri := entry.URI
						ui.Button(row.Context, "Edit").
							Label("Edit " + entry.Tag).
							OnClick(func() { a.openEditLocalNodeModal(idx, uri) })
						ui.Button(row.Context, "Delete").
							Label("Delete " + entry.Tag).
							OnClick(func() { a.deleteLocalNode(idx) })
					}
				})
			})

		listEl.Children(func() {
			if len(filtered) == 0 {
				msg := fmt.Sprintf("No nodes in %s yet.", groupName)
				if a.query != "" {
					msg = "No matching nodes."
				}
				ui.Column(c).Fill().Center().Padding(24).Children(func() {
					ui.Text(c, msg).TextColor(t.TextMuted)
				})
			}
		})

		if listEl.Submitted() && a.selectedNode >= 0 && a.selectedNode < len(filtered) {
			connectIdx = filtered[a.selectedNode].Index
		}
	})

	switch {
	case stopRequested:
		a.stopRunner()
	case connectIdx >= 0:
		a.connectNode(groupName, connectIdx)
	case editIdx >= 0:
		a.openEditLocalNodeModal(editIdx, editURI)
	case deleteIdx >= 0:
		a.deleteLocalNode(deleteIdx)
	}
}
