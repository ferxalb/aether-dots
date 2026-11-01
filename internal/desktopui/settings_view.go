package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) settingsView(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "Make it yours").FontSize(27).Bold()
			ui.Text(c, "Tune generation, save skills, and set local schedules.").TextColor(theme.TextMuted)
		})
		a.openAISettings(c, theme)
		a.modelOptionsSettings(c, theme)
		a.skillLibrarySettings(c, theme)
		a.routineSettings(c, theme)
		a.appearanceSettings(c, theme)
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			save := ui.PrimaryButton(c.Key("settings-save"), "Save changes").Padding(10, 16).
				Disabled(a.settingsSaving || a.modelConnection == "Checking…")
			if save.Clicked() {
				a.saveSettings()
			}
			if a.settingsSaving {
				ui.Text(c, "Saving securely…").FontSize(12).TextColor(theme.TextMuted)
			}
			ui.Spacer(c)
			ui.Text(c, "Preferences stay on this Mac").FontSize(11).TextColor(theme.TextMuted)
		})
	})
}
