package desktopui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
)

func (a *app) openRouterModelSettings(c *ui.Context, theme *ui.Theme, modelName *string) {
	ui.Column(c).FillWidth().Gap(7).Children(func() {
		ui.Text(c, "Model").FontSize(11).Bold()
		selectedName := strings.TrimSpace(*modelName)
		if selectedName == "" {
			selectedName = "Choose a model"
		} else {
			for _, item := range a.openRouterCatalog {
				if item.ID == strings.TrimSpace(*modelName) {
					selectedName = item.Name
					break
				}
			}
		}
		button := ui.Button(c.Key("settings-openrouter-model-open"), "").FillWidth().Padding(10, 12).
			Gap(10).Radius(10).Background(theme.Background).Border(1, theme.Border).
			Disabled(a.settingsSaving || a.modelConnection == "Checking…")
		button.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, selectedName).FontSize(12).Bold().FillWidth().SingleLine()
				if strings.TrimSpace(*modelName) != "" {
					ui.Text(c, *modelName).FontSize(10).TextColor(theme.TextMuted).FillWidth().SingleLine()
				}
			})
			ui.Text(c, "Browse models…").FontSize(11).TextColor(theme.Accent)
		})
		if button.Clicked() {
			a.openRouterModelDraft = *modelName
			a.openRouterModelPickerOpen = true
		}
		if metadata, ok := a.selectedOpenRouterModel(); ok {
			tools := "Native functions unavailable"
			if metadata.SupportsTools {
				tools = "Native functions supported"
			}
			ui.Text(c, fmt.Sprintf("Model context: %s tokens · %s", modelContextLabel(metadata.ContextLength), tools)).
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		}
		ui.Text(c, "OpenRouter’s text model catalog is available from the model picker. Replies use the model’s default token limit unless you set an optional cap.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
	})
}

func (a *app) openRouterModelPickerModal(c *ui.Context, theme *ui.Theme, modelName *string) {
	ui.Modal(c, &a.openRouterModelPickerOpen, func() {
		results := a.openRouterModelResults()
		a.openRouterModelList.Key = func(i int) any { return results[i].ID }
		panel := ui.Column(c).Width(680).MaxWidth(740).MaxHeight(720).Gap(12)
		panel.Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, "Choose an OpenRouter model").FontSize(18).Bold()
					ui.Text(c, "Search text chat models by name or model ID.").FontSize(11).TextColor(theme.TextMuted)
				})
				if ui.Button(c.Key("settings-openrouter-model-close"), "Close").Padding(7, 10).Clicked() {
					a.openRouterModelPickerOpen = false
				}
			})
			ui.Divider(c)
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				button := ui.Button(c.Key("settings-openrouter-model-catalog"), "").Grow(1).Padding(9, 11).
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
			} else if len(a.openRouterCatalog) == 0 && !a.openRouterCatalogLoaded {
				ui.Text(c, "Enter an OpenRouter API key in Settings before loading the catalog.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			if len(a.openRouterCatalog) > 0 {
				ui.TextInput(c.Key("settings-openrouter-model-search"), &a.openRouterSearch).
					Placeholder("Search all text chat models").FillWidth().Label("Search models").
					AutoFocus().ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
			}
			modelField := ui.TextInput(c.Key("settings-provider-model-openrouter"), &a.openRouterModelDraft).
				Placeholder("Or enter an exact OpenRouter model ID").FillWidth().Label("Model ID").
				ReadOnly(a.settingsSaving || a.modelConnection == "Checking…")
			if modelField.Changed() {
				a.modelConnection = "Not tested"
			}
			if metadata, ok := a.openRouterModelByID(a.openRouterModelDraft); ok {
				tools := "No native functions"
				if metadata.SupportsTools {
					tools = "Native functions supported"
				}
				ui.Text(c, fmt.Sprintf("Context: %s tokens · %s", modelContextLabel(metadata.ContextLength), tools)).
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			ui.List(c.Key("settings-openrouter-model-list"), &a.openRouterModelList, len(results), func(i int) {
				item := results[i]
				selected := item.ID == strings.TrimSpace(a.openRouterModelDraft)
				row := ui.Button(c.Key("model-select"), "").
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
					a.openRouterModelDraft = item.ID
					a.openRouterModelPickerOpen = false
				}
			}).FillWidth().MaxHeight(380).MinHeight(0).Gap(7)
			if len(a.openRouterCatalog) > 0 && len(results) == 0 {
				ui.Text(c, "No models match that search.").FontSize(12).TextColor(theme.TextMuted).Padding(14, 8)
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c.Key("settings-openrouter-model-cancel"), "Cancel").Padding(8, 12).Clicked() {
					a.openRouterModelPickerOpen = false
				}
				useID := ui.PrimaryButton(c.Key("settings-openrouter-model-use"), "Use this model ID").Padding(8, 12)
				useID.Disabled(strings.TrimSpace(a.openRouterModelDraft) == "" || a.settingsSaving || a.modelConnection == "Checking…")
				if useID.Clicked() {
					a.setOpenRouterModel(a.openRouterModelDraft)
					a.openRouterModelDraft = a.prefs.OpenRouterModel
					a.openRouterModelPickerOpen = false
				}
			})
		})
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
	a.setOpenRouterModel(item.ID)
	a.applyOpenRouterModelMetadata(item)
}

func (a *app) setOpenRouterModel(id string) {
	id = strings.TrimSpace(id)
	a.prefs.OpenRouterModel = id
	a.prefs.OpenRouterCatalogModel = ""
	a.prefs.OpenRouterModelContextLength = 0
	a.prefs.OpenRouterModelSupportsTools = false
	for _, item := range a.openRouterCatalog {
		if item.ID == id {
			a.applyOpenRouterModelMetadata(item)
			break
		}
	}
	a.modelConnection = "Not tested"
}

func (a *app) applyOpenRouterModelMetadata(item model.OpenRouterModel) {
	a.prefs.OpenRouterCatalogModel = item.ID
	a.prefs.OpenRouterModelContextLength = item.ContextLength
	a.prefs.OpenRouterModelSupportsTools = item.SupportsTools
}

func (a *app) selectedOpenRouterModel() (model.OpenRouterModel, bool) {
	return a.openRouterModelByID(a.prefs.OpenRouterModel)
}

func (a *app) openRouterModelByID(id string) (model.OpenRouterModel, bool) {
	selected := strings.TrimSpace(id)
	if selected == "" {
		return model.OpenRouterModel{}, false
	}
	for _, item := range a.openRouterCatalog {
		if item.ID == selected {
			return item, true
		}
	}
	if a.prefs.OpenRouterCatalogModel == selected {
		return model.OpenRouterModel{
			ID: selected, ContextLength: a.prefs.OpenRouterModelContextLength,
			SupportsTools: a.prefs.OpenRouterModelSupportsTools,
		}, true
	}
	return model.OpenRouterModel{}, false
}

func (a *app) openRouterModelResults() []model.OpenRouterModel {
	query := strings.ToLower(strings.TrimSpace(a.openRouterSearch))
	filtered := make([]model.OpenRouterModel, 0, len(a.openRouterCatalog))
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
