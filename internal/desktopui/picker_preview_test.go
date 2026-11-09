package desktopui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func TestWriteModelPickerPreview(t *testing.T) {
	dir := os.Getenv("AETHER_UI_PREVIEW_DIR")
	if dir == "" {
		t.Skip("set AETHER_UI_PREVIEW_DIR")
	}
	_ = os.MkdirAll(dir, 0o755)
	chat, _, _ := newUIPreviewApps(t)
	a := newSettingsPreviewApp(t, chat)
	a.settingsTab = 1
	a.prefs.Provider = string(model.ProviderOpenRouter)
	a.prefs.OpenRouterModel = "meta/muse-spark-1.3-contributor"
	a.openRouterModelPickerOpen = true
	for _, empty := range []bool{false, true} {
		if !empty {
			for i := 0; i < 14; i++ {
				a.openRouterCatalog = append(a.openRouterCatalog, model.OpenRouterModel{
					ID: "vendor/model-" + string(rune('a'+i)), Name: "Vendor: Model " + string(rune('A'+i)),
					ContextLength: 128000, SupportsTools: i%2 == 0, ReasoningMetadataKnown: true, SupportsReasoning: i%3 == 0})
			}
			a.openRouterCatalogLoaded = true
		} else {
			a.openRouterCatalog = nil
			a.openRouterCatalogLoaded = false
		}
		for _, mode := range []string{"Dark", "Light"} {
			p := *a
			p.prefs.Appearance = mode
			name := "picker-" + mode
			if empty {
				name += "-empty"
			}
			f, _ := os.Create(filepath.Join(dir, name+".png"))
			_ = png.Encode(f, ui.Render(p.view, 1120, 760, 1))
			f.Close()
		}
	}
}
