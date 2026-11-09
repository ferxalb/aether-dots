package desktopui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aether-dots/internal/model"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

func (a *app) openRouterModelSettings(c *ui.Context, theme *ui.Theme, modelName *string) {
	settingsRow(c, theme, "Model", "Browse OpenRouter's live text-model catalog.", func() {
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
		button := ui.Button(c.Key("settings-openrouter-model-open"), "").FillWidth().Padding(9, 12).
			Gap(10).Radius(10).Background(theme.Background).Border(1, theme.Border).
			Label("Browse models").Disabled(a.settingsSaving || a.modelConnection == "Checking…")
		if button.Hovered() {
			button.Background(theme.SurfaceHover)
		}
		button.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, selectedName).FontSize(13).Bold().FillWidth().SingleLine()
				if strings.TrimSpace(*modelName) != "" && selectedName != strings.TrimSpace(*modelName) {
					ui.Text(c, *modelName).FontSize(10).TextColor(theme.TextMuted).FillWidth().SingleLine()
				}
			})
			ui.Text(c, "Change").FontSize(11).TextColor(theme.Accent)
		})
		if button.Clicked() {
			a.openRouterModelDraft = *modelName
			a.openRouterModelPickerOpen = true
		}
		if metadata, ok := a.selectedOpenRouterModel(); ok {
			tools := "No native functions"
			if metadata.SupportsTools {
				tools = "Native functions"
			}
			reasoning := "Default reasoning"
			if metadata.ReasoningMetadataKnown && metadata.SupportsReasoning {
				reasoning = "Configurable reasoning"
			} else if !metadata.ReasoningMetadataKnown {
				reasoning = "Reasoning unknown"
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, chip := range []string{modelContextLabel(metadata.ContextLength) + " context", tools, reasoning} {
					ui.Text(c, chip).FontSize(10).TextColor(theme.TextMuted).Padding(2, 8).Radius(6).
						Background(theme.Background).Border(1, theme.Border)
				}
			})
		}
	})
}

// modelPickerFrost blurs what the window drew behind the picker and tints it.
var modelPickerFrost = shader.MustCompile(`package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	b := backdropSample(pos)
	// Drain the color first so bright accents don't bloom into halos.
	l := b.r*0.299 + b.g*0.587 + b.b*0.114
	g := mix(b.rgb, vec3(l, l, l), 0.85)
	return vec4(mix(g, Tint.rgb, Tint.a), 1)
}
`)

func (a *app) openRouterModelPickerModal(c *ui.Context, theme *ui.Theme, modelName *string) {
	ui.DialogBase(c, &a.openRouterModelPickerOpen, func(backdrop, panel ui.Element) {
		busy := a.settingsSaving || a.modelConnection == "Checking…"
		tint := ui.RGBA(0, 0, 0, 0.62)
		if !theme.Dark {
			tint = ui.RGBA(16, 20, 28, 0.42)
		}
		backdrop.Draw(func(p *ui.Painter, r ui.Rect) {
			modelPickerFrost.Draw(p, r, shader.Options{BackdropBlur: 6, Uniforms: map[string]any{"Tint": tint}})
		})
		// Fixed size: the dialog never grows or shrinks as the catalog loads or the search narrows.
		panel.Width(540).MaxWidthPercent(92).Height(520).MaxHeightPercent(88).
			Radius(18).Background(theme.Background).Border(1, theme.Border).Clip().
			Shadow(0, 18, 48, 0, ui.RGBA(0, 0, 0, 0.35))

		results := a.openRouterModelResults()
		a.openRouterModelList.Key = func(i int) any { return results[i].ID }
		hasCatalog := len(a.openRouterCatalog) > 0

		ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			// Header
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(18, 20, 12, 22).Shrink(0).Children(func() {
				providerLogo(c, string(model.ProviderOpenRouter), 32)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, "Choose a model").FontSize(16).Bold()
					subtitle := "Browse OpenRouter's text models."
					if hasCatalog {
						subtitle = fmt.Sprintf("%d text models · search by name or ID", len(a.openRouterCatalog))
					}
					ui.Text(c, subtitle).FontSize(11).TextColor(theme.TextMuted)
				})
				close := appIconButton(c, theme, "settings-openrouter-model-close", "Close", iconClose)
				if close.Clicked() {
					a.openRouterModelPickerOpen = false
				}
			})
			// Search + refresh
			if hasCatalog {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(0, 20, 12, 22).Shrink(0).Children(func() {
					ui.TextInput(c.Key("settings-openrouter-model-search"), &a.openRouterSearch).
						Placeholder("Search models").Grow(1).Label("Search models").AutoFocus().ReadOnly(busy)
					refreshLabel := "Refresh"
					if a.openRouterCatalogLoading {
						refreshLabel = "Loading…"
					}
					refresh := appButton(c, theme, "settings-openrouter-model-catalog", refreshLabel, buttonSecondary).Shrink(0).
						Disabled(a.openRouterCatalogLoading || busy)
					if refresh.Clicked() {
						a.refreshOpenRouterModelCatalog()
					}
				})
			}
			settingsDivider(c, theme)
			// Body: the list, or an empty state, always filling the same space.
			ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
				if !hasCatalog {
					ui.Column(c).FillWidth().Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(10).Padding(24).Children(func() {
						switch {
						case a.openRouterCatalogLoading:
							ui.Text(c, "Loading OpenRouter models…").FontSize(13).TextColor(theme.TextMuted)
						case a.openRouterCatalogError != "":
							ui.Text(c, a.openRouterCatalogError).FontSize(12).TextColor(theme.Warning).FillWidth()
						default:
							ui.Text(c, "Load the catalog to browse models").FontSize(13).Bold()
							ui.Text(c, "Uses your saved OpenRouter API key. You can also type an exact model ID below.").
								FontSize(11).TextColor(theme.TextMuted).FillWidth()
						}
						load := appButton(c, theme, "settings-openrouter-model-catalog", "Load model catalog", buttonPrimary).
							Disabled(a.openRouterCatalogLoading || busy)
						if load.Clicked() {
							a.refreshOpenRouterModelCatalog()
						}
					})
					return
				}
				if len(results) == 0 {
					ui.Text(c, "No models match that search.").FontSize(12).TextColor(theme.TextMuted).Padding(24)
					return
				}
				ui.List(c.Key("settings-openrouter-model-list"), &a.openRouterModelList, len(results), func(i int) {
					item := results[i]
					selected := item.ID == strings.TrimSpace(a.openRouterModelDraft)
					row := ui.Button(c.Key("model-select"), "").FillWidth().Padding(8, 11).Radius(9).
						Background(ui.Transparent).Border(1, ui.Transparent)
					if selected {
						row.Background(theme.Accent.Alpha(0.12)).Border(1, theme.Accent.Alpha(0.5))
					} else if row.Hovered() {
						row.Background(theme.SurfaceHover)
					}
					row.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, item.Name).FontSize(12).Bold().FillWidth().SingleLine()
							ui.Text(c, item.ID).FontSize(10).TextColor(theme.TextMuted).FillWidth().SingleLine()
						})
						chips := []string{modelContextLabel(item.ContextLength)}
						if item.SupportsTools {
							chips = append(chips, "Tools")
						}
						if item.ReasoningMetadataKnown && item.SupportsReasoning {
							chips = append(chips, "Reasoning")
						}
						ui.Row(c).Shrink(0).Gap(5).Children(func() {
							for _, chip := range chips {
								ui.Text(c, chip).FontSize(10).TextColor(theme.TextMuted).Padding(2, 7).Radius(6).
									Background(theme.Surface).Border(1, theme.Border)
							}
						})
					})
					if row.Clicked() {
						a.selectOpenRouterModel(item)
						a.openRouterModelDraft = item.ID
						a.openRouterModelPickerOpen = false
					}
				}).FillWidth().Grow(1).MinHeight(0).Gap(2).Padding(8, 12)
			})
			settingsDivider(c, theme)
			// Footer: exact ID + actions.
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Padding(12, 20, 14, 22).Shrink(0).Children(func() {
				modelField := ui.TextInput(c.Key("settings-provider-model-openrouter"), &a.openRouterModelDraft).
					Placeholder("Or paste an exact model ID").Grow(1).MinWidth(0).Label("Model ID").ReadOnly(busy)
				if modelField.Changed() {
					a.syncConnectionState()
				}
				if appButton(c, theme, "settings-openrouter-model-cancel", "Cancel", buttonSecondary).Shrink(0).Clicked() {
					a.openRouterModelPickerOpen = false
				}
				useID := appButton(c, theme, "settings-openrouter-model-use", "Use model", buttonPrimary).Shrink(0)
				useID.Disabled(strings.TrimSpace(a.openRouterModelDraft) == "" || busy)
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
			a.prefs.OpenRouterModelSupportsReasoning = false
			a.prefs.OpenRouterModelSupportsReasoningSet = false
			a.prefs.OpenRouterModelSupportedEfforts = ""
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
	a.prefs.OpenRouterModelSupportsReasoning = false
	a.prefs.OpenRouterModelSupportsReasoningSet = false
	a.prefs.OpenRouterModelSupportedEfforts = ""
	for _, item := range a.openRouterCatalog {
		if item.ID == id {
			a.applyOpenRouterModelMetadata(item)
			break
		}
	}
	a.syncConnectionState()
}

func (a *app) applyOpenRouterModelMetadata(item model.OpenRouterModel) {
	a.prefs.OpenRouterCatalogModel = item.ID
	a.prefs.OpenRouterModelContextLength = item.ContextLength
	a.prefs.OpenRouterModelSupportsTools = item.SupportsTools
	a.prefs.OpenRouterModelSupportsReasoning = item.SupportsReasoning
	a.prefs.OpenRouterModelSupportsReasoningSet = item.ReasoningMetadataKnown
	a.prefs.OpenRouterModelSupportedEfforts = strings.Join(item.SupportedReasoningEfforts, ",")
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
		var efforts []string
		if a.prefs.OpenRouterModelSupportedEfforts != "" {
			efforts = strings.Split(a.prefs.OpenRouterModelSupportedEfforts, ",")
		}
		return model.OpenRouterModel{
			ID: selected, ContextLength: a.prefs.OpenRouterModelContextLength,
			SupportsTools:             a.prefs.OpenRouterModelSupportsTools,
			SupportsReasoning:         a.prefs.OpenRouterModelSupportsReasoning,
			ReasoningMetadataKnown:    a.prefs.OpenRouterModelSupportsReasoningSet,
			SupportedReasoningEfforts: efforts,
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
