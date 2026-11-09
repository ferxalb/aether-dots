package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) settingsView(c *ui.Context, theme *ui.Theme) {
	section := max(0, min(len(a.settingsScroll)-1, a.settingsTab))
	sectionKey := [...]string{"general", "ai", "integrations", "skills", "routines", "mcp"}[section]
	heading, description := "Preferences", "Choose how AETHER looks and how your Dots work."
	if section == 2 {
		heading = "Integrations"
		description = "Connect the tools your Dots can work with. You're always in control of what they can access."
	} else if section == 5 {
		heading = "MCP servers"
		description = "Find local MCP servers automatically and choose which Dots can use their tools."
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Gap(14).Padding(20, 26, 0, 26).Shrink(0).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, heading).FontSize(24).Bold()
					ui.Text(c, description).
						FontSize(12).TextColor(theme.TextMuted).FillWidth()
				})
				a.settingsAction(c, theme)
			})
			ui.Tabs(c, &a.settingsTab, "General", "AI & responses", "Integrations", "Skills", "Routines", "MCP").FillWidth()
		})
		ui.Scroll(c.Key("settings-content-" + sectionKey)).TrackScroll(&a.settingsScroll[section]).
			Grow(1).Shrink(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().MinWidth(0).Gap(16).Padding(18, 26, 24, 26).Children(func() {
				switch a.settingsTab {
				case 1:
					a.providerSettings(c, theme)
					a.advancedResponseSettings(c, theme)
					if a.settingsAdvancedOpen {
						a.modelOptionsSettings(c, theme)
					}
				case 2:
					a.integrationSettings(c, theme)
				case 3:
					a.skillLibrarySettings(c, theme)
				case 4:
					a.routineSettings(c, theme)
				case 5:
					a.mcpSettingsPage(c, theme)
				default:
					a.appearanceSettings(c, theme)
				}
			})
		})
	})
}

func (a *app) settingsAction(c *ui.Context, theme *ui.Theme) {
	switch a.settingsTab {
	case 0, 1:
		switch {
		case a.settingsSaving:
			ui.Text(c, "Saving securely…").FontSize(11).TextColor(theme.TextMuted)
		case a.settingsDirty():
			ui.Text(c, "Unsaved changes").FontSize(11).TextColor(theme.TextMuted)
			save := ui.PrimaryButton(c.Key("settings-save"), "Save changes").Padding(9, 14).
				Disabled(a.modelConnection == "Checking…")
			if save.Clicked() {
				a.saveSettings()
			}
		default:
			ui.Icon(c, iconCheck).Size(13, 13).TextColor(theme.Success)
			ui.Text(c, "All changes saved").FontSize(11).TextColor(theme.TextMuted)
		}
	case 3:
		if a.skillEditorOpen {
			label := "Save skill"
			if a.skillEditingID != "" {
				label = "Save changes"
			}
			if ui.PrimaryButton(c.Key("settings-save-skill"), label).Padding(9, 14).Clicked() {
				a.saveSkill()
			}
		}
	case 4:
		if a.routineEditorOpen {
			label := "Save routine"
			if a.routineEditingID != "" {
				label = "Save changes"
			}
			if ui.PrimaryButton(c.Key("settings-save-routine"), label).Padding(9, 14).Clicked() {
				a.saveRoutine()
			}
		}
	}
}

func (a *app) advancedResponseSettings(c *ui.Context, theme *ui.Theme) {
	label := "Show advanced response options"
	if a.settingsAdvancedOpen {
		label = "Hide advanced response options"
	}
	button := ui.Button(c.Key("settings-advanced-toggle"), "").FillWidth().Padding(13, 15).
		Gap(12).Radius(12).Background(theme.Surface).Border(1, theme.Border).
		Label(label)
	if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
	button.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, "Advanced response options").FontSize(12).Bold()
			ui.Text(c, "Adjust thinking effort, reply length, and what happens when a request fails.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		chevron := "⌄"
		if a.settingsAdvancedOpen {
			chevron = "⌃"
		}
		ui.Text(c, chevron).FontSize(14).TextColor(theme.TextMuted)
	})
	if button.Clicked() {
		a.settingsAdvancedOpen = !a.settingsAdvancedOpen
	}
}
