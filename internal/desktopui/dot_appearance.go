package desktopui

import (
	"strconv"
	"strings"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

const (
	defaultDotIcon  = "✦"
	defaultDotColor = "#7458D8"
)

type dotShape struct {
	id, name, fallback string
	body, face         *ui.SVG
}

var dotShapes = []dotShape{
	newDotShape("dot:orbit", "Orbit", "●",
		"M16 1.6c8 0 14.4 6.4 14.4 14.4S24 30.4 16 30.4 1.6 24 1.6 16 8 1.6 16 1.6Z",
		"M16 7.6c.8-.5 1.8-.3 2.4.5l2 2.8a1.7 1.7 0 0 1-2.8 2l-2-2.8c-.6-.8-.4-1.9.4-2.5Zm5.3-1.1c.6-.4 1.4-.3 1.9.3l1.7 2.3a1.3 1.3 0 0 1-2.1 1.6L21 8.4c-.4-.6-.3-1.5.3-1.9Z"),
	newDotShape("dot:pebble", "Pebble", "◕",
		"M5 7c3-5 9-6 15-4 6 2 10 6 10 12 0 6-3 13-9 15-5 1-13-2-17-6C0 19 2 12 5 7Z",
		"M18 6.4c1-.2 1.9.4 2.1 1.3l.8 3.6a1.8 1.8 0 0 1-3.5.8l-.8-3.6c-.2-1 .4-1.9 1.4-2.1Zm5.5 1.3c.8-.2 1.5.3 1.7 1.1l.6 2.7a1.4 1.4 0 0 1-2.8.6l-.6-2.7c-.2-.8.3-1.5 1.1-1.7Z"),
	newDotShape("dot:comet", "Comet", "▶",
		"M7 3c3-2 8 0 13 3 6 4 10 7 10 11 0 5-7 8-13 11-5 2-11 4-14 0-3-4 0-22 4-25Z",
		"M13 7c.7-.6 1.7-.6 2.4.1l2.7 2.7a1.7 1.7 0 1 1-2.4 2.4L13 9.5c-.7-.7-.7-1.8 0-2.5Zm5.7.8c.5-.5 1.3-.5 1.8 0l2.2 2.1a1.3 1.3 0 0 1-1.8 1.9l-2.2-2.2c-.5-.5-.5-1.3 0-1.8Z"),
	newDotShape("dot:sprout", "Sprout", "✿",
		"M4 10c3-5 7-5 11-3 1-4 4-6 8-4 5 2 8 9 7 14-1 7-7 13-14 13C8 30 2 25 2 19c0-4 1-7 2-9Z",
		"M13 11c1-.2 1.9.4 2.1 1.3l.7 3.1a1.8 1.8 0 0 1-3.5.8l-.7-3.1c-.2-1 .4-1.9 1.4-2.1Zm5.4-1.6c.8-.2 1.5.3 1.7 1.1l.6 2.8a1.4 1.4 0 1 1-2.8.6l-.6-2.8c-.2-.8.3-1.5 1.1-1.7Z"),
	newDotShape("dot:puff", "Puff", "✹",
		"M7 7c2-4 6-6 10-4 4-1 8 1 9 5 4 2 5 6 3 10 1 5-2 8-6 9-3 4-8 4-12 2-4 0-8-2-8-7-3-3-2-8 1-11l3-4Z",
		"M15 8.4c1-.3 1.9.2 2.2 1.1l1.1 3a1.8 1.8 0 0 1-3.4 1.2l-1-3c-.4-.9.1-1.9 1.1-2.3Zm5.5-.9c.7-.2 1.5.1 1.8.9l.9 2.4a1.4 1.4 0 0 1-2.6 1l-.9-2.5c-.3-.8.1-1.6.8-1.8Z"),
}

var dotColors = []struct{ name, hex string }{
	{"Mint", "#51B8AA"}, {"Apricot", "#FFA347"}, {"Periwinkle", "#7774FA"},
	{"Violet", "#A27AFF"}, {"Blue", "#5095FF"}, {"Coral", "#FF8055"},
	{"Rose", "#F57C9E"}, {"Lime", "#B8D772"},
}

func newDotShape(id, name, fallback, body, face string) dotShape {
	path := func(data string) *ui.SVG {
		return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><path fill="currentColor" d="` + data + `"/></svg>`))
	}
	return dotShape{id, name, fallback, path(body), path(face)}
}

// The original default pair follows the Dot's identity. Existing custom
// glyphs and colors continue to use the values saved in the profile.
func dotAppearance(id, icon, color string) (string, string) {
	if (icon == "" || icon == defaultDotIcon) && (color == "" || strings.EqualFold(color, defaultDotColor)) {
		var hash uint32 = 2166136261
		for i := 0; i < len(id); i++ {
			hash = (hash ^ uint32(id[i])) * 16777619
		}
		return dotShapes[hash%uint32(len(dotShapes))].id,
			dotColors[(hash>>8)%uint32(len(dotColors))].hex
	}
	return icon, color
}

func dotProfileAvatar(c *ui.Context, icon, color string, size float32) {
	if len(color) != 7 || color[0] != '#' {
		color = defaultDotColor
	} else if _, err := strconv.ParseUint(color[1:], 16, 24); err != nil {
		color = defaultDotColor
	}
	background := ui.Hex(color)
	ink := contrastInk(background)
	for _, shape := range dotShapes {
		if icon == shape.id {
			ui.Box(c).Size(size, size).Shrink(0).Children(func() {
				ui.Icon(c, shape.body).Size(size, size).TextColor(background)
				ui.Icon(c, shape.face).Absolute().Top(0).Left(0).Size(size, size).TextColor(ink)
			})
			return
		}
	}
	if icon == "" {
		icon = defaultDotIcon
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 2).
		Background(background).Center().Children(func() {
		ui.Text(c, icon).SingleLine().FontSize(size * 0.44).Bold().TextColor(ink)
	})
}

// Phone clients render the icon field as text.
func mobileDotIcon(icon string) string {
	for _, shape := range dotShapes {
		if icon == shape.id {
			return shape.fallback
		}
	}
	return icon
}

func profileAvatarForConversation(c *ui.Context, conversation task.Conversation, size float32) {
	icon, color := dotAppearance(conversation.ID, conversation.Icon, conversation.Color)
	dotProfileAvatar(c, icon, color, size)
}

func (a *app) profileAppearance(c *ui.Context, theme *ui.Theme, id, icon, color string) {
	profilePanel(c, theme, "Appearance", func() {
		ui.Text(c, "Shape").FontSize(11).TextColor(theme.TextMuted)
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, shape := range dotShapes {
				button := ui.Button(c.Key("dot-shape-"+shape.id), "").Size(50, 44).Padding(0).
					Radius(workspaceControlRadius).Label(shape.name).Tooltip(shape.name).Center()
				styleAppearanceChoice(button, theme, shape.id == icon)
				button.Children(func() { dotProfileAvatar(c, shape.id, color, 27) })
				button.OnClick(func() { a.setDotAppearance(id, shape.id, color) })
			}
		})
		ui.Text(c, "Color").FontSize(11).TextColor(theme.TextMuted)
		ui.Row(c).Wrap().Gap(7).Children(func() {
			for _, swatch := range dotColors {
				button := ui.Button(c.Key("dot-color-"+swatch.hex), "").Size(28, 28).Padding(3).
					Radius(14).Label(swatch.name).Tooltip(swatch.name).Center()
				styleAppearanceChoice(button, theme, strings.EqualFold(swatch.hex, color))
				button.Children(func() {
					ui.Box(c).Size(18, 18).Radius(9).Background(ui.Hex(swatch.hex))
				})
				button.OnClick(func() { a.setDotAppearance(id, icon, swatch.hex) })
			}
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Text(c, "Custom hex").FontSize(11).TextColor(theme.TextMuted)
			ui.TextInput(c.Key("dot-custom-color"), &a.profileColorInput).
				Label("Custom avatar color").Placeholder("#RRGGBB").Width(96).OnChange(func() {
				a.profileIcon, a.profileColor = icon, a.profileColorInput
			})
			ui.Spacer(c)
			appButton(c, theme, "dot-reset-appearance", "Reset", buttonGhost).
				Tooltip("Restore automatic avatar").Disabled(a.profileIcon == defaultDotIcon && a.profileColor == defaultDotColor).
				OnClick(func() { a.setDotAppearance(id, defaultDotIcon, defaultDotColor) })
		})
	})
}

func (a *app) setDotAppearance(id, icon, color string) {
	a.profileIcon, a.profileColor = icon, color
	_, a.profileColorInput = dotAppearance(id, icon, color)
}

func styleAppearanceChoice(button ui.Element, theme *ui.Theme, selected bool) {
	button.Background(theme.Surface).Border(1, theme.Border).Selected(selected)
	if button.Hovered() {
		button.Background(theme.SurfaceHover)
	}
	if selected {
		button.Border(2, theme.Text)
	}
}
