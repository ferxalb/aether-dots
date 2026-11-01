package desktopui

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

func (a *app) routinesListView(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).FillWidth().MaxWidth(760).MinWidth(0).Gap(12).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Text(c, "Saved Routines").FontSize(18).Bold()
			ui.Text(c, "Reusable task instructions keep the source and output folders from their original Dot.").
				FontSize(11).TextColor(theme.TextMuted).FillWidth()
		})
		if len(a.snapshot.Routines) == 0 {
			ui.Text(c, "No saved Routines yet. Save one from a completed Dot to reuse its fixed local grants.").
				FontSize(12).TextColor(theme.TextMuted).FillWidth().Padding(10, 0)
			return
		}
		for _, routine := range a.snapshot.Routines {
			button := ui.Button(c.Key("saved-routine-"+routine.ID), "").FillWidth().Padding(11, 12).
				Gap(10).Radius(11).Label(routine.Name)
			if button.Hovered() {
				button.Background(theme.SurfaceHover)
			}
			button.Children(func() {
				ui.Column(c).FillWidth().MinWidth(0).Gap(4).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
						ui.Text(c, routine.Name).FontSize(13).Bold().FillWidth().MinWidth(0)
						ui.Text(c, "v"+strconv.Itoa(routine.Version)).FontSize(10).TextColor(theme.TextMuted)
					})
					ui.Text(c, boundedSidebarText(routine.Objective)).FontSize(11).TextColor(theme.TextMuted).
						SingleLine().FillWidth()
				})
			})
			if button.Clicked() {
				a.selectedRoutineID = routine.ID
				a.routineName = routine.Name
				a.routineObjective = routine.Objective
			}
		}
	})
}
