package desktopui

import "github.com/egoist/mygo/ui"

func (a *app) view(c *ui.Context) {
	theme := a.theme(c)
	bar := c.TitleBar()
	dragHeight := max(bar.Height, 44)
	vibrant := c.Vibrancy() && a.prefs.Appearance == "System"
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	rootColor := theme.Background
	if vibrant {
		rootColor = ui.Transparent
	}

	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(rootColor).Children(func() {
		a.sidebar(c, theme, dragHeight, vibrant)
		ui.Box(c).Width(1).FillHeight().Background(theme.Border.Alpha(0.42))

		main := ui.Column(c).Grow(1).FillHeight().MinWidth(0).Background(rootColor)
		main.Children(func() {
			// Keep the hidden native title-bar strip clear for traffic lights and
			// window dragging. The toolbar begins below it, so controls stay live.
			ui.Box(c).FillWidth().Height(dragHeight)
			a.header(c, theme)
			ui.Scroll(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).FillWidth().MinWidth(0).Gap(18).Padding(18, 28, 32, 28).Children(func() {
					a.statusMessage(c, theme)
					a.workspaceContent(c, theme)
				})
			})
		})

		// Dragging is limited to the top inset and never overlaps the toolbar.
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(dragHeight).DragWindow()
	})
}

func (a *app) workspaceContent(c *ui.Context, theme *ui.Theme) {
	if a.settingsOpen {
		a.settingsView(c, theme)
		return
	}
	if a.selectedRoutineID != "" {
		a.routineView(c, theme)
		return
	}
	if dot, ok := findDot(a.snapshot, a.selectedDotID); ok {
		a.dotView(c, theme, dot)
		return
	}
	a.captureView(c, theme)
}

func (a *app) header(c *ui.Context, theme *ui.Theme) {
	row := ui.Row(c).FillWidth().Height(48).Padding(0, 24, 0, 22).
		AlignItems(ui.Center).Gap(12)
	row.Children(func() {
		toggle := ui.Button(c.Key("toggle-sidebar"), "").Size(30, 30).Padding(7).Radius(8).
			Label("Toggle sidebar").Tooltip("Collapse sidebar")
		icon, label := iconPanelOpen, "Collapse sidebar"
		if a.sidebarCollapsed {
			icon, label = iconPanelClosed, "Expand sidebar"
		}
		toggle.Label(label).Tooltip(label)
		if toggle.Hovered() {
			toggle.Background(theme.SurfaceHover)
		}
		toggle.Children(func() { ui.Icon(c, icon).Size(16, 16) })
		if toggle.Clicked() {
			a.sidebarCollapsed = !a.sidebarCollapsed
		}

		title := selectedSpaceName(a.snapshot, a.selectedSpaceID)
		if a.settingsOpen {
			title = "Settings"
		} else if a.selectedRoutineID != "" {
			title = "Routines"
		}
		ui.Text(c, title).SingleLine().FontSize(13).Bold().TextColor(theme.Text).
			MaxWidth(180).Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if !a.settingsOpen {
			ui.Badge(c, "⌘ ⇧ SPACE")
		}
	})
}

func (a *app) statusMessage(c *ui.Context, theme *ui.Theme) {
	if a.message == "" {
		return
	}
	status := ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(9, 12).Radius(10).
		Background(theme.Accent.Alpha(0.09)).Border(1, theme.Accent.Alpha(0.17))
	status.Children(func() {
		ui.Icon(c, iconSparkles).Size(14, 14).TextColor(theme.Accent)
		ui.Text(c, a.message).FontSize(12).TextColor(theme.Text).FillWidth()
	})
}
