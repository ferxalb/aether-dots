package desktopui

import (
	"aether-dots/internal/task"
	"github.com/egoist/mygo/ui"
)

func (a *app) dotView(c *ui.Context, theme *ui.Theme, dot task.Dot) {
	ui.Column(c).FillWidth().MaxWidth(760).MinWidth(0).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			a.conversationAvatar(c, dot.Name, dot.ID, 40)
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, dot.Name).FontSize(21).Bold()
				ui.Text(c, "Older task record · "+humanStatus(dot.Status)).FontSize(11).TextColor(theme.TextMuted)
			})
		})
		ui.Column(c).FillWidth().Gap(7).Padding(14, 16).Radius(13).
			Background(theme.Surface).Border(1, theme.Border).Children(func() {
			ui.Text(c, "Original request").FontSize(11).Bold().TextColor(theme.TextMuted)
			ui.Text(c, dot.Objective).FontSize(13).FillWidth()
		})
		ui.Column(c).FillWidth().Gap(7).Padding(14, 16).Radius(13).
			Background(theme.Accent.Alpha(0.07)).Border(1, theme.Accent.Alpha(0.15)).Children(func() {
			ui.Text(c, "This task used the retired local-folder workflow.").FontSize(12).Bold()
			ui.Text(c, "Folder permissions and output paths are no longer retained or available. Create a new Dot to continue in chat.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
			if ui.PrimaryButton(c.Key("legacy-create-dot"), "Create a new Dot").Clicked() {
				a.startCreator()
			}
		})
	})
}
