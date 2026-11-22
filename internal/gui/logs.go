package gui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *App) logsView(c *ui.Context) {
	t := c.Theme()
	lines := a.runner.lines
	compact := isCompact(c)

	renderActions := func() {
		if len(lines) > 0 && !a.logList.AtEnd() {
			if ui.Button(c, "Jump to Bottom").Clicked() {
				a.logList.ScrollToEnd()
			}
		}
		if ui.Button(c, "Copy All").Disabled(len(lines) == 0).Clicked() {
			c.Services().WriteClipboard(a.runner.allText())
			c.Toast("Copied logs")
		}
		if ui.Button(c, "Clear").Disabled(len(lines) == 0).Clicked() {
			a.runner.clear()
		}
		if ui.Button(c, "Stop").Label("Stop sing-box").Disabled(!a.runner.busy()).Clicked() {
			a.stopRunner()
		}
	}

	renderStatusBadge := func() {
		switch {
		case a.runner.starting:
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Label("Starting sing-box")
				ui.Badge(c, fmt.Sprintf("Starting · %s", a.runner.activeTag)).
					Background(t.Warning).
					TextColor(t.AccentText)
			})
		case a.runner.active:
			ui.Badge(c, fmt.Sprintf("Running · %s", a.runner.activeTag)).
				Background(t.Success).
				TextColor(t.AccentText)
		default:
			ui.Badge(c, "Stopped")
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
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Logs").FontSize(18).Bold()
					renderStatusBadge()
				})
				ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(renderActions)
			})
		} else {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Logs").FontSize(18).Bold()
				renderStatusBadge()
				ui.Spacer(c)
				renderActions()
			})
		}

		a.errorBannerView(c)

		if a.runner.active {
			a.metricsBarView(c)
		}

		a.logList.Key = func(i int) any { return lines[i].id }
		a.logList.FollowEnd = true

		logBox := ui.Box(c).Grow(1).Radius(t.Radius).
			Background(t.Surface).Border(1, t.Border).Clip()
		logBox.Children(func() {
			ui.List(c.Key("logs-list"), &a.logList, len(lines), func(i int) {
				entry := lines[i]
				color := t.Text
				switch entry.kind {
				case logKindStart:
					color = t.Success
				case logKindStop:
					color = t.Danger
				}
				ui.Text(c, entry.text).
					Font("monospace").
					FontSize(12).
					TextColor(color).
					Padding(2, 10).
					Selectable()
			}).Fill().Padding(8, 0).KeepScrollbars(true).Label("sing-box logs").Children(func() {
				if len(lines) == 0 {
					ui.Column(c).Fill().Center().Padding(24).Children(func() {
						ui.Text(c, "No sing-box logs yet. Select a node to connect.").
							TextColor(t.TextMuted)
					})
				}
			})
		})
	})
}

func (a *App) metricsBarView(c *ui.Context) {
	t := c.Theme()
	m := a.runner.metrics

	card := func(label, value, sub string) {
		ui.Column(c).Grow(1).MinWidth(0).Padding(8, 12).Gap(2).Radius(t.Radius).
			Background(t.Surface).Border(1, t.Border).Children(func() {
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).SingleLine()
			ui.Text(c, value).FontSize(14).Bold().FontFeatures("tnum").SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(11).TextColor(t.TextMuted).FontFeatures("tnum").SingleLine()
			}
		})
	}

	downVal, downSub := "—", "Total —"
	upVal, upSub := "—", "Total —"
	connVal, memVal := "—", "—"
	if m.ready {
		downVal = formatRate(m.downRate)
		downSub = "Total " + formatBytes(m.downTotal)
		upVal = formatRate(m.upRate)
		upSub = "Total " + formatBytes(m.upTotal)
		connVal = fmt.Sprint(m.connections)
		memVal = formatBytes(int64(m.memory))
	}

	if isCompact(c) {
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
				card("Download", downVal, downSub)
				card("Upload", upVal, upSub)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Stretch).Children(func() {
				card("Connections", connVal, "Active")
				card("Memory", memVal, "sing-box")
			})
		})
		return
	}

	ui.Row(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
		card("Download", downVal, downSub)
		card("Upload", upVal, upSub)
		card("Connections", connVal, "Active")
		card("Memory", memVal, "sing-box")
	})
}
