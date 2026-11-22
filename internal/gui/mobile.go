package gui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

const compactWidthBreakpoint = 640

func (a *App) compactView(c *ui.Context) {
	t := c.Theme()

	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Background(t.Surface).Padding(10, 14, 8, 14).Gap(8).Shrink(0).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Singer").FontSize(17).Bold().Grow(1)
				if ui.Button(c, "+").Label("Add remote").Tooltip("Add remote group").Padding(4, 10).Clicked() {
					a.openAddRemoteModal()
				}
			})

			ui.ScrollHorizontal(c).Shrink(0).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					for _, g := range a.groups {
						id := navGroupPrefix + g.Name
						selected := a.selectedNav == id
						label := fmt.Sprintf("%s (%d)", g.Name, len(g.Nodes))
						if selected {
							ui.PrimaryButton(c.Key("nav-"+id), label).
								Label(g.Name).
								Padding(5, 10).
								OnClick(func() { a.selectedNav = id })
						} else {
							ui.Button(c.Key("nav-"+id), label).
								Label(g.Name).
								Padding(5, 10).
								OnClick(func() { a.selectedNav = id })
						}
					}

					logsLabel := "Logs"
					if a.runner.active {
						logsLabel = "Logs · Running"
					}
					if a.selectedNav == navLogs {
						ui.PrimaryButton(c.Key("nav-logs"), logsLabel).
							Label("Logs").
							Padding(5, 10).
							OnClick(func() { a.selectedNav = navLogs })
					} else {
						ui.Button(c.Key("nav-logs"), logsLabel).
							Label("Logs").
							Padding(5, 10).
							OnClick(func() { a.selectedNav = navLogs })
					}
				})
			})
		})

		ui.Divider(c)

		ui.Column(c).Grow(1).MinWidth(0).FillWidth().Children(func() {
			a.mainPaneView(c)
		})

		ui.Divider(c)
		a.compactFooterView(c)
	})
}

func (a *App) compactFooterView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Background(t.Surface).Padding(10, 14).Gap(8).Shrink(0).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if a.runner.active {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.Success).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, a.runner.activeTag).FontSize(13).Bold().SingleLine()
					if m := a.runner.metrics; m.ready {
						ui.Textf(c, "↓ %s · ↑ %s", formatRate(m.downRate), formatRate(m.upRate)).
							FontSize(11).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine()
					} else {
						ui.Text(c, "Connected").FontSize(11).TextColor(t.Success).SingleLine()
					}
				})
				if ui.Button(c, "Stop").Label("Stop sing-box").Clicked() {
					a.stopRunner()
				}
			} else {
				ui.Box(c).Size(8, 8).Radius(4).Background(t.TextMuted.Alpha(0.5)).Shrink(0)
				ui.Text(c, "Disconnected").FontSize(12).TextColor(t.TextMuted).Grow(1)
			}

			label := ui.Text(c, "TUN").FontSize(12)
			if ui.Switch(c, &a.tunMode).LabelledBy(label).Changed() && a.runner.active {
				a.connectNode(a.runner.activeGroup, a.runner.activeIndex)
			}
		})
	})
}
