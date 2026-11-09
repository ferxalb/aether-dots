package desktopui

import "github.com/egoist/mygo/ui"

const (
	workspaceControlRadius = 14
	workspacePanelRadius   = 18
)

type buttonKind int

const (
	buttonSecondary buttonKind = iota // outlined; the default action style
	buttonPrimary                     // filled accent; one per surface
	buttonGhost                       // no chrome until hovered
	buttonDanger                      // ghost with destructive text
)

var iconClose = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6 6 18"/></svg>`))

// appButton is the app's one text button. Callers chain Disabled, Grow, Shrink
// and read Clicked; the look comes from kind alone, so screens stay consistent.
func appButton(c *ui.Context, theme *ui.Theme, key, label string, kind buttonKind) ui.Element {
	if kind == buttonPrimary {
		return ui.PrimaryButton(c.Key(key), label).Padding(8, 14).Radius(workspaceControlRadius)
	}
	b := ui.Button(c.Key(key), "").Padding(8, 12).Radius(workspaceControlRadius).Label(label)
	text := theme.Text
	switch kind {
	case buttonGhost:
		b.Background(ui.Transparent)
	case buttonDanger:
		b.Background(ui.Transparent)
		text = theme.Danger
	default:
		b.Background(ui.Transparent).Border(1, theme.Border)
	}
	if b.Hovered() {
		if kind == buttonDanger {
			b.Background(theme.Danger.Alpha(0.12))
		} else {
			b.Background(theme.SurfaceHover)
		}
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(text) })
	return b
}

// appIconButton is a square, centered icon-only button; label is its
// accessible name.
func appIconButton(c *ui.Context, theme *ui.Theme, key, label string, icon *ui.SVG) ui.Element {
	b := ui.Button(c.Key(key), "").Size(30, 30).Padding(8).Shrink(0).Radius(workspaceControlRadius).Label(label).
		Background(ui.Transparent).Border(1, theme.Border)
	if b.Hovered() {
		b.Background(theme.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(14, 14).TextColor(theme.TextMuted) })
	return b
}
