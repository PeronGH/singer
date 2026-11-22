package gui

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *App) logsView(c *ui.Context) {
	t := c.Theme()
	lines := a.runner.lines

	ui.Column(c).Fill().Padding(16, 20).Gap(12).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Logs").FontSize(18).Bold()
			if a.runner.active {
				ui.Badge(c, fmt.Sprintf("Running · %s", a.runner.activeTag)).
					Background(t.Success).
					TextColor(t.AccentText)
			} else {
				ui.Badge(c, "Stopped")
			}
			ui.Spacer(c)

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
			if ui.Button(c, "Stop").Label("Stop sing-box").Disabled(!a.runner.active).Clicked() {
				a.stopRunner()
			}
		})

		a.errorBannerView(c)

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
