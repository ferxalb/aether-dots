package desktopui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

func (a *app) creatorView(c *ui.Context, theme *ui.Theme) {
	ui.Row(c).FillWidth().Padding(10, 0, 0, 0).Children(func() {
		ui.Spacer(c)
		content := ui.Column(c).FillWidth().MaxWidth(760).MinWidth(0).Gap(7)
		content.Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "Create a Dot").FontSize(20).Bold()
			})

			form := ui.Column(c).FillWidth().Gap(5)
			form.Children(func() {
				ui.Field(c, "Dot name", func() {
					ui.TextInputBase(c.Key("creator-dot-name"), &a.dotName).
						Placeholder("e.g. Design review").FillWidth()
				})
				ui.Field(c, "Purpose", func() {
					ui.TextAreaBase(c.Key("creator-purpose"), &a.objective).
						Placeholder("What should this Dot help you accomplish?").
						MinHeight(40).MaxHeight(64).FillWidth()
				})
			})

			ui.Column(c).FillWidth().Gap(4).Children(func() {
				ui.Text(c, "Local access · fixed for future tasks").FontSize(12).Bold().FillWidth()
				folderPermissionRow(c, theme, "Read", "source", a.sourceRoot, "Choose a source folder", func() { a.pickFolder(true) })
				folderPermissionRow(c, theme, "Write", "output", a.outputDir, "Choose an output folder", func() { a.pickFolder(false) })
			})

			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
				ui.Text(c, "Space").FontSize(12).Bold().TextColor(theme.TextMuted)
				spaceSelect(c, theme, &a.selectedSpaceID, a.snapshot.Spaces)
				manageSpaces := "Manage spaces"
				if a.creatorSpaceManagementOpen {
					manageSpaces = "Close space management"
				}
				if ui.Button(c.Key("creator-manage-spaces"), manageSpaces).Clicked() {
					if a.creatorSpaceManagementOpen {
						a.closeSpaceManagement()
					} else {
						a.openSpaceCreate()
					}
				}
			})
			if a.creatorSpaceManagementOpen {
				ui.Column(c).FillWidth().Gap(6).Padding(10).Radius(11).
					Background(theme.Surface).Border(1, theme.Border).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						label := "New space"
						action := "Add Space"
						if a.renameSpaceID != "" {
							label = "Rename space"
							action = "Rename Space"
						}
						ui.TextInputBase(c.Key("creator-space-draft"), &a.spaceDraft).
							Placeholder(label).Label(label).Grow(1).MinWidth(120)
						if ui.Button(c.Key("creator-space-save"), action).Clicked() {
							a.saveSpaceDraft()
						}
						if a.renameSpaceID == "" && a.selectedSpaceID != "" {
							if ui.Button(c.Key("creator-space-rename-selected"), "Rename selected").Clicked() {
								a.openSpaceRename(a.selectedSpaceID, selectedSpaceName(a.snapshot, a.selectedSpaceID))
							}
						}
					})
				})
			}

			ui.Column(c).FillWidth().Gap(4).Children(func() {
				ui.Text(c, "Execution").FontSize(13).Bold()
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
					executionSelect(c, theme, &a.mode)
					ui.Text(c, creatorExecutionDisclosureFor(a.mode)).FontSize(10).TextColor(theme.TextMuted).Grow(1).MinWidth(0).FillWidth()
				})
			})

			ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Spacer(c)
				create := ui.PrimaryButton(c.Key("create-dot"), "").Gap(8).Padding(9, 14).
					Label("Create Dot").Tooltip("Create Dot and open its chat")
				create.Children(func() {
					ui.Icon(c, iconPlus).Size(15, 15)
					ui.Text(c, "Create Dot").FontSize(12).Bold()
				})
				create.Disabled(strings.TrimSpace(a.dotName) == "" || strings.TrimSpace(a.objective) == "" ||
					a.sourceRoot == "" || a.outputDir == "" || !containsSpace(a.snapshot, a.selectedSpaceID))
				if create.Clicked() {
					a.createConversationFromCreator()
				}
			})
		})
		ui.Spacer(c)
	})
}

func (a *app) workspaceWelcomeView(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Fill().Center().Padding(24, 32).Gap(14).Children(func() {
		mark := ui.Box(c).Size(52, 52).Radius(17).
			Background(theme.Accent.Alpha(0.13)).TextColor(theme.Accent)
		mark.Children(func() { ui.Icon(c, iconSparkles).Size(24, 24) })
		ui.Column(c).MaxWidth(420).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Create a Dot to begin").FontSize(20).Bold()
			ui.Text(c, "Give it a name, purpose, and local folders. Then send tasks from its chat.").
				FontSize(13).TextColor(theme.TextMuted).FillWidth().TextAlign(ui.Center)
		})
		if ui.PrimaryButton(c.Key("welcome-create-dot"), "Create a Dot").Clicked() {
			a.startCreator()
		}
	})
}

func (a *app) captureView(c *ui.Context, theme *ui.Theme) {
	a.workspaceWelcomeView(c, theme)
}
