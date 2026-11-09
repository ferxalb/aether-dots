package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) profileInspector(c *ui.Context, theme *ui.Theme) {
	ui.Column(c.Key("dot-profile-inspector")).Width(profileInspectorWidth).Shrink(0).FillHeight().
		Background(sidebarColor(theme, false, a.prefs.Palette)).Children(func() {
		ui.Row(c).FillWidth().Height(workspaceHeaderHeight).AlignItems(ui.Center).
			Padding(0, 14).DragWindow().Children(func() {
			ui.Text(c, "Dot profile").FontSize(13).Bold()
			ui.Spacer(c)
			if appIconButton(c, theme, "close-profile-inspector", "Close profile", iconClose).
				Tooltip("Close profile").Clicked() {
				a.conversationDetailsOpen = false
			}
		})
		ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.55))
		a.profileContent(c, theme)
	})
}

func (a *app) profileContent(c *ui.Context, theme *ui.Theme) {
	ui.Scroll(c.Key("profile-scroll-" + a.selectedConversationID)).
		Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(12, 12, 16, 12).Children(func() {
			a.conversationDetailsView(c, theme)
		})
	})
	ui.Box(c).FillWidth().Height(1).Background(theme.Border.Alpha(0.55))
	a.profileActions(c, theme)
}
