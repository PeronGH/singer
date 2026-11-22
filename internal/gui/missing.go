package gui

import "github.com/egoist/mygo/ui"

func (a *App) missingSingBoxView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Center().Padding(32).Children(func() {
		ui.Column(c).
			MaxWidth(440).
			Padding(24).
			Gap(12).
			Radius(t.Radius).
			Background(t.Surface).
			Border(1, t.Border).
			Children(func() {
				ui.Text(c, "sing-box not found").FontSize(20).Bold()
				ui.Text(c, "Singer requires the sing-box binary to be installed and available on your PATH.").
					TextColor(t.TextMuted)
				ui.Text(c, a.singBoxErr.Error()).
					Font("monospace").
					FontSize(12).
					TextColor(t.Danger).
					Selectable()
				ui.Row(c).Gap(8).Justify(ui.End).Margin(4, 0, 0, 0).Children(func() {
					if ui.Button(c, "Quit").Clicked() {
						a.quit()
					}
					if ui.PrimaryButton(c, "Check Again").Clicked() {
						a.checkSingBox()
					}
				})
			})
	})
}
