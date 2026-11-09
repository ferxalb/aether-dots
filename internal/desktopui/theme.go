package desktopui

import (
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

type paletteColors struct {
	background, surface, hover, pressed, border  string
	text, muted, accent, accentHover, accentDown string
	accentText                                   string
}

type themePalette struct {
	id, name    string
	light, dark paletteColors
	lightAccent string
	darkAccent  string
}

func paletteOptions() []themePalette {
	return []themePalette{
		{
			// Midnight: true black with a restrained indigo accent. Surfaces are
			// near-black with a faint cool tint, never gray.
			id: "midnight", name: "Midnight", lightAccent: "#4A5AE8", darkAccent: "#7C8BFF",
			light: paletteColors{"#F4F5F9", "#FFFFFF", "#EAECF3", "#DEE1EB", "#D5D8E3", "#14161F", "#6C7184", "#4A5AE8", "#3E4DD3", "#3341BA", "#FFFFFF"},
			dark:  paletteColors{"#000000", "#0A0A0D", "#141418", "#1B1B21", "#22222A", "#F2F3F7", "#8D909C", "#7C8BFF", "#95A1FF", "#6574EE", "#05060F"},
		},
		{
			id: "obsidian", name: "Obsidian", lightAccent: "#3A424D", darkAccent: "#D0D0D0",
			light: paletteColors{"#F3F4F6", "#FFFFFF", "#E9EBEF", "#DDE1E7", "#D4D9E0", "#252A31", "#6F7783", "#3A424D", "#303741", "#252B33", "#FFFFFF"},
			dark:  paletteColors{"#222222", "#2C2C2C", "#333333", "#3A3A3A", "#3B3B3B", "#F0F0F0", "#A0A0A0", "#D0D0D0", "#DEDEDE", "#BABABA", "#191919"},
		},
		{
			id: "forest", name: "Forest", lightAccent: "#168459", darkAccent: "#48C98D",
			light: paletteColors{"#F3F8F5", "#FFFFFF", "#EAF2ED", "#DDE9E1", "#D5E3DA", "#202A24", "#6E7D73", "#168459", "#11714C", "#0D5B3D", "#FFFFFF"},
			dark:  paletteColors{"#0E1512", "#151F19", "#1E2B22", "#26362B", "#304136", "#EDF5EF", "#9DACA1", "#48C98D", "#67D8A2", "#34B67A", "#102017"},
		},
		{
			id: "ocean", name: "Ocean", lightAccent: "#1D69C8", darkAccent: "#5BAEFF",
			light: paletteColors{"#F3F7FC", "#FFFFFF", "#E9F0FA", "#DCE7F5", "#D5E1F0", "#202A38", "#6F7E91", "#1D69C8", "#185AB0", "#124B96", "#FFFFFF"},
			dark:  paletteColors{"#0F141B", "#161E29", "#222C39", "#2A3747", "#344252", "#EDF3FA", "#A0ADBD", "#5BAEFF", "#79BEFF", "#4098EC", "#101D2A"},
		},
		{
			id: "silver", name: "Silver", lightAccent: "#718096", darkAccent: "#AAB4C4",
			light: paletteColors{"#F1F3F6", "#FFFFFF", "#E8EBF0", "#DCE1E8", "#D4DAE2", "#242932", "#737D8B", "#718096", "#626F84", "#536075", "#FFFFFF"},
			dark:  paletteColors{"#15171B", "#1D2025", "#292D33", "#32373F", "#3B414A", "#F0F2F5", "#A4ABB5", "#AAB4C4", "#BBC4D1", "#929EAF", "#1B1E23"},
		},
		{
			id: "violet", name: "Violet", lightAccent: "#6557D9", darkAccent: "#A294FF",
			light: paletteColors{"#F5F6FB", "#FFFFFF", "#EEeff8", "#E4E5F1", "#E0E2ED", "#222438", "#6E7389", "#6557D9", "#5749C8", "#493EAF", "#FFFFFF"},
			dark:  paletteColors{"#11141C", "#1B202B", "#252B39", "#303748", "#343C4E", "#F1F2FB", "#A3A9BD", "#A294FF", "#B4AAFF", "#8A79ED", "#17142B"},
		},
		{
			id: "rose", name: "Rose", lightAccent: "#C33E70", darkAccent: "#FF8CAB",
			light: paletteColors{"#FBF4F6", "#FFFFFF", "#F6E8ED", "#EEDCE3", "#EBD6DE", "#30242A", "#88717A", "#C33E70", "#AB315F", "#92274F", "#FFFFFF"},
			dark:  paletteColors{"#191117", "#241920", "#32212B", "#3B2934", "#45303C", "#F8EFF3", "#B6A0AA", "#FF8CAB", "#FFA2BA", "#ED7599", "#28151D"},
		},
		{
			id: "amber", name: "Amber", lightAccent: "#A9660B", darkAccent: "#FFC66E",
			light: paletteColors{"#FBF8F1", "#FFFFFF", "#F4ECD8", "#EBE0C7", "#E9DDC0", "#302A1E", "#887D65", "#A9660B", "#935907", "#7D4B06", "#FFFFFF"},
			dark:  paletteColors{"#19150E", "#241E14", "#332919", "#3E321F", "#493B25", "#F8F3E8", "#B6AA90", "#FFC66E", "#FFD28B", "#EDB34D", "#2A2112"},
		},
	}
}

func paletteByID(id string) themePalette {
	for _, palette := range paletteOptions() {
		if palette.id == id {
			return palette
		}
	}
	return paletteOptions()[0]
}

func (a *app) theme(c *ui.Context) *ui.Theme {
	dark := c.Theme().Dark
	switch a.prefs.Appearance {
	case "Light":
		dark = false
	case "Dark":
		dark = true
	}
	palette := paletteByID(a.prefs.Palette)
	colors := palette.light
	if dark {
		colors = palette.dark
	}
	theme := ui.LightTheme()
	if dark {
		theme = ui.DarkTheme()
	}
	theme.Background = ui.Hex(colors.background)
	theme.Surface = ui.Hex(colors.surface)
	theme.SurfaceHover = ui.Hex(colors.hover)
	theme.SurfacePressed = ui.Hex(colors.pressed)
	theme.Border = ui.Hex(colors.border)
	theme.Text = ui.Hex(colors.text)
	theme.TextMuted = ui.Hex(colors.muted)
	theme.Accent = ui.Hex(colors.accent)
	theme.AccentHover = ui.Hex(colors.accentHover)
	theme.AccentPressed = ui.Hex(colors.accentDown)
	theme.AccentText = ui.Hex(colors.accentText)
	theme.Selection = theme.Accent.Alpha(0.25)
	theme.Focus = theme.Accent.Alpha(0.62)
	theme.Scrollbar = theme.Text.Alpha(0.32)
	if c.Preferences().HighContrast {
		theme.Border = theme.Border.Mix(theme.Text, 0.48)
		theme.TextMuted = theme.TextMuted.Mix(theme.Text, 0.4)
		theme.Focus.A = 255
		theme.Scrollbar = theme.Scrollbar.Alpha(1.6)
	}
	theme.Radius = 10
	theme.Spacing = 4
	c.SetTheme(theme)
	return c.Theme()
}

func sidebarColor(theme *ui.Theme, vibrant bool, paletteID string) ui.Color {
	if paletteID == "midnight" && theme.Dark {
		return ui.Hex("#000000")
	}
	if paletteID == "obsidian" && theme.Dark {
		return ui.Hex("#141414")
	}
	if vibrant {
		return theme.Surface.Alpha(0.76)
	}
	return theme.Surface
}

func sidebarSection(c *ui.Context, theme *ui.Theme, icon *ui.SVG, label string) {
	ui.Icon(c, icon).Size(13, 13).TextColor(theme.TextMuted)
	ui.Text(c, label).FontSize(11).Bold().TextColor(theme.TextMuted)
}

func styleSidebarItem(button ui.Element, theme *ui.Theme, selected bool) {
	button.Background(ui.Transparent).Border(1, ui.Transparent).TextColor(theme.Text)
	if selected {
		button.Background(theme.Accent.Alpha(0.12)).Border(1, theme.Accent.Alpha(0.17)).TextColor(theme.Text)
	} else if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
}

func statusIcon(status task.Status) *ui.SVG {
	switch status {
	case task.Done:
		return iconCheck
	case task.Running:
		return iconPlay
	case task.Failed:
		return iconAlert
	case task.Paused:
		return iconPause
	case task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return iconClock
	default:
		return iconPlay
	}
}

func statusColor(theme *ui.Theme, status task.Status) ui.Color {
	switch status {
	case task.Done:
		return theme.Success
	case task.Running:
		return theme.Accent
	case task.Failed:
		return theme.Danger
	case task.Paused, task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return theme.Warning
	default:
		return theme.TextMuted
	}
}
