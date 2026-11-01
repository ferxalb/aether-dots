package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) appearanceSettings(c *ui.Context, theme *ui.Theme) {
	card := ui.Column(c).FillWidth().Gap(15).Padding(18).Radius(16).
		Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "Appearance").FontSize(15).Bold()
				ui.Text(c, "A comfortable canvas for your work.").FontSize(12).TextColor(theme.TextMuted)
			})
			ui.Spacer(c)
			ui.Select(c.Key("settings-appearance"), &a.prefs.Appearance, []string{"System", "Light", "Dark"}).Width(220).
				Disabled(a.settingsSaving)
		})
		ui.Divider(c)
		ui.Column(c).Gap(9).Children(func() {
			ui.Text(c, "Color theme").FontSize(13).Bold()
			ui.Text(c, "Each palette is tuned for both light and dark appearance.").FontSize(11).TextColor(theme.TextMuted)
		})
		palettes := paletteOptions()
		for start := 0; start < len(palettes); start += 4 {
			end := min(start+4, len(palettes))
			ui.Row(c).Gap(9).Children(func() {
				for _, palette := range palettes[start:end] {
					a.paletteButton(c, theme, palette)
				}
			})
		}
	})
}

func (a *app) paletteButton(c *ui.Context, theme *ui.Theme, palette themePalette) {
	button := ui.Button(c.Key("theme-"+palette.id), "").Grow(1).MinWidth(0).Height(44).
		Padding(8, 10).Gap(9).Radius(11).Label(palette.name).
		Background(theme.Background).Border(1, theme.Border).Disabled(a.settingsSaving)
	if a.prefs.Palette == palette.id {
		button.Background(theme.Accent.Alpha(0.10)).Border(1.5, theme.Accent.Alpha(0.75))
	}
	button.Children(func() {
		swatch := palette.lightAccent
		if a.prefs.Appearance == "Dark" || (a.prefs.Appearance == "System" && c.Theme().Dark) {
			swatch = palette.darkAccent
		}
		ui.Box(c).Size(19, 19).Shrink(0).Radius(10).Background(ui.Hex(swatch))
		ui.Text(c, palette.name).FontSize(12).SingleLine()
	})
	if button.Clicked() {
		a.prefs.Palette = palette.id
	}
}
