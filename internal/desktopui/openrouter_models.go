package desktopui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

const openRouterVisibleModels = 24

func (a *app) openRouterModelSettings(c *ui.Context, theme *ui.Theme, modelName *string) {
	results := a.openRouterModelResults()
	shownResults := results[:min(len(results), openRouterVisibleModels)]
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Text(c, "Model").FontSize(11).Bold()
		ui.Text(c, "Load OpenRouter’s live text chat models, then search by model name or ID.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			button := ui.Button(c.Key("settings-openrouter-model-catalog"), "").Grow(1).Padding(8, 10).
				Radius(9).Background(theme.Background).Border(1, theme.Border).
				Disabled(a.openRouterCatalogLoading || a.settingsSaving || a.modelConnection == "Checking…")
			button.Children(func() {
				label := "Load model catalog"
				if a.openRouterCatalogLoaded {
					label = "Refresh model catalog"
				}
				if a.openRouterCatalogLoading {
					label = "Loading OpenRouter models…"
				}
				ui.Text(c, label).FontSize(12)
			})
			if button.Clicked() {
				a.refreshOpenRouterModelCatalog()
			}
			if len(a.openRouterCatalog) > 0 {
				ui.Text(c, fmt.Sprintf("%d models", len(a.openRouterCatalog))).FontSize(11).TextColor(theme.TextMuted)
			}
		})
		if a.openRouterCatalogError != "" {
			ui.Text(c, a.openRouterCatalogError).FontSize(11).TextColor(theme.Warning).FillWidth()
		}
		if len(a.openRouterCatalog) == 0 && a.openRouterCatalogError == "" && !a.openRouterCatalogLoaded {
			ui.Text(c, "OpenRouter requires an API key to read the catalog. Enter a key above, then load the catalog; the key is sent only to OpenRouter over HTTPS.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		if len(a.openRouterCatalog) == 0 && a.openRouterCatalogLoaded {
			ui.Text(c, "OpenRouter returned no text chat models. You can still enter an exact model ID.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		if len(a.openRouterCatalog) > 0 {
			ui.TextInput(c.Key("settings-openrouter-model-search"), &a.openRouterSearch).
				Placeholder("Search all text chat models").FillWidth().Label("Search models").
				ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
		}
		modelField := ui.TextInput(c.Key("settings-provider-model-openrouter"), modelName).
			Placeholder("Choose a model or enter its exact OpenRouter ID").
			FillWidth().Label("OpenRouter model ID").
			ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
		if modelField.Changed() {
			a.modelConnection = "Not tested"
		}
		if metadata, ok := a.selectedOpenRouterModel(); ok {
			tools := "Native functions unavailable"
			if metadata.SupportsTools {
				tools = "Native functions supported"
			}
			ui.Text(c, fmt.Sprintf("Model context: %s tokens · %s", modelContextLabel(metadata.ContextLength), tools)).
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		for i, item := range shownResults {
			selected := item.ID == *modelName
			row := ui.Button(c.Key(fmt.Sprintf("settings-openrouter-model-%d", i)), "").
				FillWidth().Padding(9, 11).Radius(9).Background(theme.Background).Border(1, theme.Border)
			if selected {
				row.Background(theme.Accent.Alpha(0.12)).Border(1, theme.Accent)
			}
			row.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, item.Name).FontSize(12).Bold().FillWidth().SingleLine()
					ui.Text(c, item.ID).FontSize(10).TextColor(theme.TextMuted).FillWidth().SingleLine()
				})
				ui.Column(c).Shrink(0).Gap(3).Children(func() {
					ui.Text(c, modelContextLabel(item.ContextLength)+" ctx").FontSize(10).TextColor(theme.TextMuted)
					capability := "No functions"
					if item.SupportsTools {
						capability = "Functions"
					}
					ui.Text(c, capability).FontSize(10).TextColor(theme.TextMuted)
				})
			})
			if row.Clicked() {
				a.selectOpenRouterModel(item)
			}
		}
		if len(results) > openRouterVisibleModels {
			ui.Text(c, fmt.Sprintf("Showing %d of %d matches. Search to narrow the list.", openRouterVisibleModels, len(results))).
				FontSize(10).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Text(c, "Context size and function support come from each model’s OpenRouter catalog entry. Manually entered IDs may not have catalog metadata. Reply tokens are left to the model unless you set an optional limit below.").
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	})
}

func (a *app) refreshOpenRouterModelCatalog() {
	if a.openRouterCatalogLoading || a.prefs.Provider != string(model.ProviderOpenRouter) {
		return
	}
	a.openRouterCatalogRequest++
	requestID := a.openRouterCatalogRequest
	a.openRouterCatalogLoading = true
	a.openRouterCatalogError = ""
	draftKey := strings.TrimSpace(a.apiKeyDraft)
	win := a.win
	go func() {
		key := draftKey
		if key == "" {
			key = providerAPIKey(model.ProviderOpenRouter)
		}
		var catalog []model.OpenRouterModel
		var err error
		if key == "" {
			err = fmt.Errorf("enter an OpenRouter API key, save one in the system credential store, or set OPENROUTER_API_KEY")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			catalog, err = model.FetchOpenRouterModels(ctx, key)
			cancel()
		}
		update := func() {
			if a.openRouterCatalogRequest != requestID || a.prefs.Provider != string(model.ProviderOpenRouter) {
				return
			}
			a.openRouterCatalogLoading = false
			a.openRouterCatalogLoaded = err == nil
			if err != nil {
				a.openRouterCatalogError = err.Error()
				return
			}
			a.openRouterCatalogError = ""
			a.openRouterCatalog = catalog
			a.prefs.OpenRouterCatalogModel = ""
			a.prefs.OpenRouterModelContextLength = 0
			a.prefs.OpenRouterModelSupportsTools = false
			for _, item := range catalog {
				if item.ID == strings.TrimSpace(a.prefs.OpenRouterModel) {
					a.applyOpenRouterModelMetadata(item)
					break
				}
			}
		}
		if win != nil {
			win.Update(update)
		} else {
			update()
		}
	}()
}

func (a *app) selectOpenRouterModel(item model.OpenRouterModel) {
	a.prefs.OpenRouterModel = item.ID
	a.applyOpenRouterModelMetadata(item)
	a.modelConnection = "Not tested"
}

func (a *app) applyOpenRouterModelMetadata(item model.OpenRouterModel) {
	a.prefs.OpenRouterCatalogModel = item.ID
	a.prefs.OpenRouterModelContextLength = item.ContextLength
	a.prefs.OpenRouterModelSupportsTools = item.SupportsTools
}

func (a *app) selectedOpenRouterModel() (model.OpenRouterModel, bool) {
	selected := strings.TrimSpace(a.prefs.OpenRouterModel)
	if selected == "" || a.prefs.OpenRouterCatalogModel != selected {
		return model.OpenRouterModel{}, false
	}
	for _, item := range a.openRouterCatalog {
		if item.ID == selected {
			return item, true
		}
	}
	return model.OpenRouterModel{
		ID: selected, ContextLength: a.prefs.OpenRouterModelContextLength,
		SupportsTools: a.prefs.OpenRouterModelSupportsTools,
	}, true
}

func (a *app) openRouterModelResults() []model.OpenRouterModel {
	query := strings.ToLower(strings.TrimSpace(a.openRouterSearch))
	filtered := make([]model.OpenRouterModel, 0, min(len(a.openRouterCatalog), openRouterVisibleModels))
	for _, item := range a.openRouterCatalog {
		if query != "" && !strings.Contains(strings.ToLower(item.Name), query) && !strings.Contains(strings.ToLower(item.ID), query) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func modelContextLabel(tokens int64) string {
	if tokens <= 0 {
		return "Unknown"
	}
	if tokens%1_000_000 == 0 {
		return fmt.Sprintf("%dM", tokens/1_000_000)
	}
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	}
	if tokens%1000 == 0 {
		return fmt.Sprintf("%dk", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}
