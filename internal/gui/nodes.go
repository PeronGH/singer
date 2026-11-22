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

	compact := isCompact(c)
	fetching := isRemote && a.fetching[groupName]
	renderGroupActions := func() {
		if isLocal {
			if ui.PrimaryButton(c, "Add Node").Clicked() {
				a.openAddLocalNodeModal()
			}
		}
		if isRemote {
			refreshLabel := "Refresh"
			if fetching {
				refreshLabel = "Fetching…"
			}
			if ui.Button(c, refreshLabel).Disabled(fetching).Clicked() {
				a.startFetchRemote(groupName, remoteURL)
			}
			if len(a.remoteGroups()) > 1 {
				if ui.Button(c, "Refresh All").Clicked() {
					a.refreshAllRemotes()
				}
			}
			if ui.Button(c, "Delete").Label("Delete remote " + groupName).Disabled(fetching).Clicked() {
				a.confirmDeleteRemoteName = groupName
				a.confirmDeleteRemoteOpen = true
			}
		}
	}

	renderGroupHeading := func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, groupName).FontSize(18).Bold().SingleLine()
			if fetching {
				ui.Badge(c, "Fetching…").Background(t.Accent.Alpha(0.15)).TextColor(t.Accent)
			}
		})
		if isRemote && remoteURL != "" {
			ui.Text(c, remoteURL).FontSize(12).TextColor(t.TextMuted).SingleLine()
		}
	}

	root := ui.Column(c).Fill().Gap(12)
	if compact {
		root.Padding(12)
	} else {
		root.Padding(16, 20)
	}
	root.Children(func() {
		if compact {
			ui.Column(c).Gap(8).Children(func() {
				ui.Column(c).MinWidth(0).Gap(2).Children(renderGroupHeading)
				ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(renderGroupActions)
			})
		} else {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(renderGroupHeading)
				renderGroupActions()
			})
		}

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

		if fetching && len(g.Nodes) > 0 {
			ui.Row(c).Padding(8, 12).Gap(8).Radius(t.Radius).
				Background(t.Surface).Border(1, t.Border).
				AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Label("Fetching " + groupName)
				ui.Text(c, "Fetching nodes from remote…").FontSize(12).TextColor(t.TextMuted)
			})
		}

		a.nodeList.Selected = &a.selectedNode
		a.nodeList.Key = func(i int) any { return filtered[i].Index }
		a.nodeList.Label = func(i int) string { return filtered[i].Tag }

		listEl := ui.List(c.Key("nodes-"+groupName), &a.nodeList, len(filtered)).
			Grow(1).
			KeepScrollbars(true).
			Label(groupName + " nodes").
			Rows(func(row ui.ListRow) {
				entry := filtered[row.Index]
				starting := a.runner.isNodeStarting(groupName, entry.Index)
				active := a.runner.isNodeActive(groupName, entry.Index)
				runningOrStarting := starting || active
				selected := row.Selected()

				textColor := t.Text
				mutedColor := t.TextMuted
				if selected && row.ListFocused() {
					textColor = t.AccentText
					mutedColor = t.AccentText.Alpha(0.8)
				}

				var rowBox ui.Element
				if compact {
					rowBox = ui.Column(row.Context).
						Padding(10, 10).
						Gap(8).
						FillWidth()
				} else {
					rowBox = ui.Row(row.Context).
						Padding(10, 12).
						Gap(10).
						AlignItems(ui.Center).
						FillWidth()
				}

				if rowBox.RowDoubleClicked() {
					connectIdx = entry.Index
				}

				rowBox.ContextMenu(func(m *ui.Menu) {
					if runningOrStarting {
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

				renderRowActions := func() {
					if runningOrStarting {
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
				}

				rowBox.Children(func() {
					info := ui.Column(row.Context).MinWidth(0).Gap(2)
					if !compact {
						info.Grow(1)
					}
					info.Children(func() {
						ui.Row(row.Context).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(row.Context, entry.Tag).Bold().TextColor(textColor).SingleLine()
							switch {
							case starting:
								ui.Badge(row.Context, "Starting…").Background(t.Warning).TextColor(t.AccentText)
							case active:
								ui.Badge(row.Context, "Active").Background(t.Success).TextColor(t.AccentText)
							}
						})
						ui.Text(row.Context, entry.Desc).FontSize(12).TextColor(mutedColor).SingleLine()
					})

					if compact {
						ui.Row(row.Context).Gap(6).Wrap().AlignItems(ui.Center).Children(renderRowActions)
					} else {
						renderRowActions()
					}
				})
			})

		listEl.Children(func() {
			if len(filtered) == 0 {
				ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
					if fetching {
						ui.Spinner(c).Label("Fetching " + groupName)
						ui.Text(c, "Fetching nodes…").TextColor(t.TextMuted)
						return
					}
					msg := fmt.Sprintf("No nodes in %s yet.", groupName)
					if a.query != "" {
						msg = "No matching nodes."
					}
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
