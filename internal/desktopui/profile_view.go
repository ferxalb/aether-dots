package desktopui

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

var dotIcons = []string{"✦", "✧", "◈", "◉", "✳", "◆", "✹", "◌"}
var dotColors = []string{"#7458D8", "#168459", "#1D69C8", "#C33E70", "#D27C31", "#37839A"}

func (a *app) conversationDetailsView(c *ui.Context, theme *ui.Theme) {
	_, ok := findConversation(a.snapshot, a.selectedConversationID)
	if !ok {
		a.workspaceWelcomeView(c, theme)
		return
	}
	a.profileDelegateIDs = validProfileDelegateIDs(a.snapshot, a.selectedConversationID, a.profileSpaceID, a.profileDelegateIDs)
	ui.Column(c).FillWidth().MaxWidth(760).MinWidth(0).Gap(18).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			dotProfileAvatar(c, a.profileIcon, a.profileColor, 52)
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "Customize your Dot").FontSize(23).Bold()
				ui.Text(c, "Set a clear role and working style. You can refine these details in chat at any time.").
					FontSize(12).TextColor(theme.TextMuted).FillWidth()
			})
		})

		if a.conversationError != "" {
			ui.Text(c, a.conversationError).FontSize(11).TextColor(theme.Danger).FillWidth()
		}

		profilePanel(c, theme, "Identity", func() {
			ui.Field(c, "Name", func() {
				ui.TextInputBase(c.Key("profile-name"), &a.profileName).
					Placeholder("e.g. Research partner").FillWidth()
			})
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "Icon").FontSize(11).Bold()
				ui.Row(c).Wrap().Gap(7).Children(func() {
					for i, icon := range dotIcons {
						icon := icon
						button := ui.Button(c.Key("profile-icon-"+string(rune('a'+i))), "").
							Size(36, 36).Radius(11).Label("Choose icon " + icon)
						button.Background(theme.Surface).Border(1, theme.Border)
						if a.profileIcon == icon {
							button.Background(theme.Accent.Alpha(0.14)).Border(1, theme.Accent)
						}
						button.Center().Children(func() { ui.Text(c, icon).FontSize(18) })
						if button.Clicked() {
							a.profileIcon = icon
						}
					}
				})
			})
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, "Color").FontSize(11).Bold()
				ui.Row(c).Wrap().Gap(8).Children(func() {
					for i, color := range dotColors {
						color := color
						button := ui.Button(c.Key("profile-color-"+string(rune('a'+i))), "").
							Size(26, 26).Radius(13).Label("Choose color " + color)
						button.Background(ui.Hex(color)).Border(2, theme.Surface)
						if a.profileColor == color {
							button.Border(3, theme.Text)
						}
						if button.Clicked() {
							a.profileColor = color
						}
					}
				})
			})
		})

		profilePanel(c, theme, "Role", func() {
			ui.Field(c, "Objective", func() {
				ui.TextAreaBase(c.Key("profile-objective"), &a.profilePurpose).
					Placeholder("What outcome should this Dot own?").
					MinHeight(70).MaxHeight(130).FillWidth()
			})
			ui.Text(c, "Give the Dot one clear area of responsibility. A prompt in chat can start the work right away.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			ui.Field(c, "Working instructions", func() {
				ui.TextAreaBase(c.Key("profile-instructions"), &a.profileInstructions).
					Placeholder("Tone, format, process, boundaries, or preferences to follow each time…").
					MinHeight(84).MaxHeight(160).FillWidth()
			})
		})

		profilePanel(c, theme, "Organization", func() {
			ui.Field(c, "Space", func() {
				profileSpaceSelect(c, theme, a)
			})
			manageSpaces := "Manage spaces"
			if a.spaceManagementOpen {
				manageSpaces = "Close space management"
			}
			if ui.Button(c.Key("profile-manage-spaces"), manageSpaces).Clicked() {
				if a.spaceManagementOpen {
					a.closeSpaceManagement()
				} else {
					a.openSpaceCreate()
				}
			}
			if a.spaceManagementOpen {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
					label, action := "New space", "Add Space"
					if a.renameSpaceID != "" {
						label, action = "Rename space", "Rename Space"
					}
					ui.TextInputBase(c.Key("profile-space-draft"), &a.spaceDraft).
						Placeholder(label).Label(label).Grow(1).MinWidth(140)
					if ui.Button(c.Key("profile-space-save"), action).Clicked() {
						a.saveSpaceDraft()
					}
					if a.renameSpaceID == "" && a.profileSpaceID != "" {
						if ui.Button(c.Key("profile-space-rename"), "Rename selected").Clicked() {
							a.openSpaceRename(a.profileSpaceID, selectedSpaceName(a.snapshot, a.profileSpaceID))
						}
					} else if a.renameSpaceID != "" {
						if ui.Button(c.Key("profile-space-cancel-rename"), "Cancel").Clicked() {
							a.closeSpaceManagement()
						}
					}
				})
				ui.Text(c, "Shared Space notes are sent with chat context to Dots in this Space. Add only information you want those Dots to use.").
					FontSize(10).TextColor(theme.TextMuted).FillWidth()
				ui.TextAreaBase(c.Key("profile-space-context"), &a.spaceContextDraft).
					Placeholder("Shared project facts, terminology, or preferences…").
					MinHeight(60).MaxHeight(120).FillWidth().Label("Shared Space notes")
				if a.spaceContextTargetID != "" && a.spaceContextTargetID == a.profileSpaceID {
					if ui.Button(c.Key("profile-space-context-save"), "Save selected Space notes").Clicked() {
						a.saveSpaceContextDraft()
					}
				}
			}
		})

		profilePanel(c, theme, "Chat memory", func() {
			ui.Text(c, "This editable summary travels with recent messages to reduce repeated context. The complete transcript remains saved on this device.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			ui.TextAreaBase(c.Key("profile-context-summary"), &a.profileContextSummary).
				Placeholder("Durable facts, decisions, preferences, and open work…").
				MinHeight(90).MaxHeight(180).FillWidth().Label("Saved Dot memory")
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				conversation, _ := findConversation(a.snapshot, a.selectedConversationID)
				compacting := a.summaryCompactRun[conversation.ID]
				buttonLabel := "Update memory from recent chat"
				if compacting {
					buttonLabel = "Updating memory…"
				}
				button := ui.Button(c.Key("profile-compact-history"), buttonLabel).Padding(8, 11).
					Tooltip("Sends recent chat context to the selected model to draft compact, editable memory.").
					Disabled(compacting || len(conversation.Messages) == 0)
				if button.Clicked() {
					a.compactConversationHistory()
				}
				ui.Spacer(c)
				remaining := task.MaxConversationSummaryBytes - len(a.profileContextSummary)
				if remaining < 0 {
					remaining = 0
				}
				ui.Text(c, fmt.Sprintf("%d bytes left", remaining)).FontSize(10).TextColor(theme.TextMuted)
			})
		})

		profilePanel(c, theme, "Specialist access", func() {
			ui.Text(c, "Choose which Dots this one may consult. Each handoff sends only the task, constraints, and expected output the model supplies. The receiving Dot keeps its own profile and chat history.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			available := 0
			for _, peer := range a.snapshot.Conversations {
				if peer.SpaceID != a.profileSpaceID || peer.ID == a.selectedConversationID {
					continue
				}
				available++
				selected := containsID(a.profileDelegateIDs, peer.ID)
				label := compactProfileText(peer.Name, 48)
				if peer.Purpose != "" {
					label += " · " + compactProfileText(peer.Purpose, 72)
				}
				if selected {
					label = "✓  " + label
				} else {
					label = "＋  " + label
				}
				button := ui.Button(c.Key("profile-delegate-"+peer.ID), label).FillWidth().Padding(8, 10).Radius(9)
				if selected {
					button.Background(theme.Accent.Alpha(0.12)).Border(1, theme.Accent.Alpha(0.4))
				} else {
					button.Background(theme.Background).Border(1, theme.Border)
				}
				if button.Clicked() {
					a.profileDelegateIDs = toggleString(a.profileDelegateIDs, peer.ID, task.MaxDelegatedDots)
				}
			}
			if available == 0 {
				ui.Text(c, "Create another Dot in this Space to make it available as a specialist.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			if len(a.profileDelegateIDs) == task.MaxDelegatedDots {
				ui.Text(c, "This Dot has reached the 12 specialist limit.").FontSize(10).TextColor(theme.TextMuted)
			}
		})

		profilePanel(c, theme, "GitHub access", func() {
			conversation, _ := findConversation(a.snapshot, a.selectedConversationID)
			ui.Text(c, "GitHub permissions are specific to this Dot and stay limited to repositories you selected in Integrations.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			if conversation.SpaceID != a.profileSpaceID {
				ui.Text(c, "Moving a Dot to another Space clears its GitHub grants. Save the new Space first, then grant repositories again.").
					FontSize(11).TextColor(theme.Danger).FillWidth()
			}
			if !a.snapshot.GitHubConnected {
				ui.Text(c, "Connect GitHub and select repositories in Settings → Integrations before granting access.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			}
			if ui.Checkbox(c.Key("profile-github-enabled"), &a.profileGitHubEnabledDraft, "Enable GitHub for this Dot").
				Disabled(!a.snapshot.GitHubConnected || conversation.SpaceID != a.profileSpaceID).Changed() && !a.profileGitHubEnabledDraft {
				a.profileGitHubRepositoryIDs = nil
				a.profileGitHubDisclosureAccepted = false
			}
			if a.profileGitHubEnabledDraft && a.snapshot.GitHubConnected && conversation.SpaceID == a.profileSpaceID {
				ui.Text(c, "Permissions · Read-only").FontSize(10).Bold().TextColor(theme.Accent)
				ui.Text(c, "Selected repository metadata, issue and pull request text, commit details, and CI results may be sent to the AI provider configured for this Dot. This release does not retrieve source files. AETHER never sends your GitHub token.").
					FontSize(10).TextColor(theme.TextMuted).FillWidth()
				if !a.profileGitHubDisclosureAccepted || len(a.profileGitHubRepositoryIDs) == 0 {
					if ui.Checkbox(c.Key("profile-github-disclosure"), &a.profileGitHubDisclosureAccepted,
						"I understand selected GitHub data may be sent to my configured AI provider").Changed() {
						// The saved grant is committed only after this explicit disclosure choice.
					}
				}
				selected := make([]task.GitHubRepository, 0)
				for _, repository := range a.snapshot.GitHubRepositories {
					if repository.Selected {
						selected = append(selected, repository)
					}
				}
				ui.TextInput(c.Key("profile-github-repository-search"), &a.profileGitHubRepositorySearch).
					Placeholder("Filter selected repositories").FillWidth().Label("Filter selected repositories")
				filtered := filterGitHubRepositories(selected, a.profileGitHubRepositorySearch)
				if len(filtered) == 0 {
					ui.Text(c, "Select repositories in Settings → Integrations first.").FontSize(10).TextColor(theme.TextMuted).FillWidth()
				} else {
					limit := a.profileGitHubRepositoryLimit
					if limit <= 0 {
						limit = 50
					}
					shown := min(len(filtered), limit)
					for _, repository := range filtered[:shown] {
						repository := repository
						checked := containsInt64(a.profileGitHubRepositoryIDs, repository.ID)
						if ui.Checkbox(c.Key("profile-github-repo-"+fmt.Sprint(repository.ID)), &checked, repository.FullName).FillWidth().Changed() {
							a.profileGitHubRepositoryIDs = toggleInt64(a.profileGitHubRepositoryIDs, repository.ID, task.MaxGitHubDotGrants)
						}
					}
					if shown < len(filtered) {
						if ui.Button(c.Key("profile-github-repos-more"), fmt.Sprintf("Show %d more", min(50, len(filtered)-shown))).Clicked() {
							a.profileGitHubRepositoryLimit = shown + 50
						}
					}
					ui.Text(c, fmt.Sprintf("%d of %d selected repositories granted to this Dot", len(a.profileGitHubRepositoryIDs), len(selected))).
						FontSize(10).TextColor(theme.TextMuted).FillWidth()
				}
				if githubToolsKnownUnsupported(a.prefs) {
					ui.Text(c, "The selected OpenRouter model is marked as not supporting function calls. This Dot won't inspect GitHub data until you select a model with function support.").
						FontSize(10).TextColor(theme.Danger).FillWidth()
				}
			}
		})

		profilePanel(c, theme, "Privacy", func() {
			ui.Text(c, "This Dot uses its profile, chat history, and any skill you name. It cannot access your files, folders, or apps. GitHub access is limited to the repositories explicitly granted above; other account access remains unavailable. Specialist access is limited to the Dots selected above, and each handoff is saved in both conversations.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth()
		})

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(9).Children(func() {
			back := ui.Button(c.Key("profile-cancel"), "Back to chat").Padding(9, 12)
			if back.Clicked() {
				a.conversationDetailsOpen = false
			}
			ui.Spacer(c)
			if ui.PrimaryButton(c.Key("profile-save"), "Save profile").Padding(9, 14).Clicked() {
				a.saveConversationProfile()
			}
		})
	})
}

func toggleString(values []string, value string, maxItems int) []string {
	for i, current := range values {
		if current == value {
			return append(values[:i], values[i+1:]...)
		}
	}
	if len(values) >= maxItems {
		return values
	}
	return append(values, value)
}

func containsID(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func toggleInt64(values []int64, target int64, maxItems int) []int64 {
	for i, value := range values {
		if value == target {
			return append(values[:i], values[i+1:]...)
		}
	}
	if len(values) >= maxItems {
		return values
	}
	return append(values, target)
}

func compactProfileText(value string, maxBytes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && end < len(value) && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end] + "…"
}

func profilePanel(c *ui.Context, theme *ui.Theme, title string, children func()) {
	ui.Column(c).FillWidth().Gap(12).Padding(14, 16).Radius(14).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Text(c, title).FontSize(12).Bold()
		children()
	})
}

func dotProfileAvatar(c *ui.Context, icon, color string, size float32) {
	if icon == "" {
		icon = "✦"
	}
	if color == "" {
		color = "#7458D8"
	}
	background := ui.Hex(color)
	ink := contrastInk(background)
	avatar := ui.Box(c).Size(size, size).Shrink(0).Radius(size * 0.34).
		Background(background).TextColor(ink)
	avatar.Center().Children(func() { ui.Text(c, icon).FontSize(size * 0.44).Bold().TextColor(ink) })
}

func contrastInk(background ui.Color) ui.Color {
	linear := func(channel uint8) float64 {
		value := float64(channel) / 255
		if value <= 0.04045 {
			return value / 12.92
		}
		return math.Pow((value+0.055)/1.055, 2.4)
	}
	luminance := 0.2126*linear(background.R) + 0.7152*linear(background.G) + 0.0722*linear(background.B)
	if luminance > 0.179 {
		return ui.Hex("#171923")
	}
	return ui.Hex("#FFFFFF")
}

func profileAvatarForConversation(c *ui.Context, conversation task.Conversation, size float32) {
	dotProfileAvatar(c, conversation.Icon, conversation.Color, size)
}

func profileSpaceSelect(c *ui.Context, theme *ui.Theme, a *app) {
	options := make([]dropdownOption, 0, len(a.snapshot.Spaces))
	for _, space := range a.snapshot.Spaces {
		options = append(options, dropdownOption{value: space.ID, label: space.Name})
	}
	dropdownSelect(c, theme, "profile-space", "Space", &a.profileSpaceID, options, false)
}

func (a *app) workspaceWelcomeView(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Fill().Center().Padding(28, 32).Gap(18).Children(func() {
		mark := ui.Box(c).Size(58, 58).Radius(19).
			Background(theme.Accent.Alpha(0.12)).TextColor(theme.Accent)
		mark.Center().Children(func() { ui.Icon(c, iconSparkles).Size(26, 26) })
		ui.Column(c).MaxWidth(440).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "A clear space for focused work").FontSize(22).Bold()
			ui.Text(c, "Create a Dot for a role you return to. Give it a name, a clear objective, and instructions that fit the way you work.").
				FontSize(13).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
		})
		if ui.PrimaryButton(c.Key("welcome-create-dot"), "New Dot").Padding(10, 16).Clicked() {
			a.startCreator()
		}
		ui.Text(c, "Dots use their profile and chat history. Named skills and local routines add reusable workflows. Dots do not access files or folders.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
	})
}
