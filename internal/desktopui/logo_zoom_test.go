package desktopui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestWriteLogoZoom(t *testing.T) {
	dir := os.Getenv("AETHER_UI_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set AETHER_UI_PREVIEW_DIR")
	}
	_ = os.MkdirAll(dir, 0o755)
	view := func(c *ui.Context) {
		ui.Row(c).Fill().Gap(12).Padding(12).Background(ui.Hex("#151A17")).Children(func() {
			for _, p := range []string{"openai", "anthropic", "xai", "openrouter", "openai_compatible"} {
				providerLogo(c, p, 40)
			}
		})
	}
	_ = model.ProviderOpenAI
	f, _ := os.Create(filepath.Join(dir, "logos.png"))
	_ = png.Encode(f, ui.Render(view, 280, 64, 4))
	f.Close()
}
