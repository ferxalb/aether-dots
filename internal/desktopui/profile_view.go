package desktopui

import (
	"math"

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
			}
		})

		profilePanel(c, theme, "Privacy", func() {
			ui.Text(c, "This Dot uses its profile, chat history, and any skill you name. It cannot access your files, folders, apps, or accounts.").
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
