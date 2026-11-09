package desktopui

import (
	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func logoSVG(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Simple geometric marks, one per provider; they are not the vendors' trademarks.
var (
	logoOpenAI     = logoSVG(`<path d="M12 3.5 19 7.5v9L12 20.5 5 16.5v-9L12 3.5Z"/><path d="M12 3.5v8m0 0 7 4m-7-4-7 4"/>`)
	logoAnthropic  = logoSVG(`<path d="M12 4v16M4.5 8l15 8M4.5 16l15-8"/>`)
	logoXAI        = logoSVG(`<path d="m5 5 14 14M19 5 5 19"/>`)
	logoOpenRouter = logoSVG(`<circle cx="5" cy="12" r="1.7" fill="currentColor" stroke="none"/><path d="M7 12h2c3 0 3-6 6-6h2M7 12h2c3 0 3 6 6 6h2"/><path d="m16 3.8 3.2 2.2L16 8.2m0 7.6 3.2 2.2-3.2 2.2"/>`)
	logoCompatible = logoSVG(`<path d="m8.5 7-5 5 5 5m7-10 5 5-5 5M13.5 5l-3 14"/>`)
)

func providerLogo(c *ui.Context, provider string, size float32) {
	icon, bg := logoOpenAI, ui.Hex("#10A37F")
	switch model.ProviderID(provider) {
	case model.ProviderAnthropic:
		icon, bg = logoAnthropic, ui.Hex("#D97757")
	case model.ProviderXAI:
		icon, bg = logoXAI, ui.Hex("#2B2B2F")
	case model.ProviderOpenRouter:
		icon, bg = logoOpenRouter, ui.Hex("#6366F1")
	case model.ProviderCompatible:
		icon, bg = logoCompatible, ui.Hex("#64748B")
	}
	// Explicit padding centers the glyph in the tile.
	glyph := size * 0.56
	ui.Row(c).Size(size, size).Padding((size - glyph) / 2).Shrink(0).Radius(size * 0.3).Background(bg).TextColor(ui.Hex("#FFFFFF")).Children(func() {
		ui.Icon(c, icon).Size(glyph, glyph)
	})
}
