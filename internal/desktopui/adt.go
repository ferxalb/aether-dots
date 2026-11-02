package desktopui

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

const (
	adtDefaultDuration = 6 * time.Second
	adtExpandDelay     = 150 * time.Millisecond
	adtCollapseBuffer  = 2 * time.Second
	adtPillHeight      = float32(40)
	adtStackGap        = float32(9)
)

var (
	adtSpring = ui.Animation{Spring: ui.SpringOptions{Duration: 600 * time.Millisecond, Bounce: 0.25}}
	adtLoader = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M21 12a9 9 0 1 1-6.2-8.6"/></svg>`))
)

// ADTLevel gives a notice its semantic color and icon.
type ADTLevel string

const (
	ADTInfo    ADTLevel = "info"
	ADTSuccess ADTLevel = "success"
	ADTWarning ADTLevel = "warning"
	ADTError   ADTLevel = "error"
	ADTLoading ADTLevel = "loading"
	ADTAction  ADTLevel = "action"
)

// ADTNotice is one transient notification. Reusing its ID updates a visible
// notice in place, which lets a loading pill morph into a result.
type ADTNotice struct {
	ID          string   // Stable IDs update an existing notice in place.
	Title       string   // Short title shown in the compact pill and expanded card.
	Description string   // Optional detail revealed by the dynamic expansion.
	Level       ADTLevel // Semantic level selects the icon and theme accent.

	Duration       time.Duration // Zero uses the default; negative keeps it until dismissed.
	ExpandAfter    time.Duration // Zero uses the default automatic expansion delay.
	CollapseBefore time.Duration // Zero collapses this long before dismissal.
	ManualExpand   bool          // Disable automatic expansion while keeping hover expansion.

	Action    string
	OnAction  func()
	Icon      *ui.SVG // Optional icon override.
	Fill      ui.Color
	TextColor ui.Color
	Roundness float32

	createdAt time.Time
}

// ADTConfig places the stack within its window. Insets leave room for
// persistent UI such as the sidebar and title bar.
type ADTConfig struct {
	Position ADTPosition // One of the four corners or two vertical center positions.
	MaxWidth float32     // Width of an expanded card; the compact pill sizes to its title.
	Top      float32     // Insets are measured from the window edges; zero uses the default.
	Right    float32
	Bottom   float32
	Left     float32
}

type ADTPosition uint8

const (
	ADTTopRight ADTPosition = iota
	ADTTopLeft
	ADTBottomRight
	ADTBottomLeft
	ADTTopCenter
	ADTBottomCenter
)

// ADT queues and renders theme-aware native notifications. Keep it in app
// state; never store a UI Context or Element here.
type ADT struct {
	pending   []ADTNotice
	hoveredID string
}

// Push queues a notice for the next view pass. Reusing an ID replaces a
// pending notice or updates its visible toast and restarts its lifetime.
func (a *ADT) Push(notice ADTNotice) string {
	if a == nil || strings.TrimSpace(notice.Title) == "" {
		return ""
	}
	if notice.Level == "" {
		notice.Level = ADTInfo
	}
	if notice.ID == "" {
		notice.ID = "aether-adt-default"
	}
	notice.Title = strings.TrimSpace(notice.Title)
	notice.Description = strings.TrimSpace(notice.Description)
	for i := range a.pending {
		if a.pending[i].ID == notice.ID {
			a.pending[i] = notice
			return notice.ID
		}
	}
	a.pending = append(a.pending, notice)
	return notice.ID
}

func (a *ADT) Info(title, description string) string {
	return a.Push(ADTNotice{Title: title, Description: description, Level: ADTInfo})
}

func (a *ADT) Success(title, description string) string {
	return a.Push(ADTNotice{Title: title, Description: description, Level: ADTSuccess})
}

func (a *ADT) Warning(title, description string) string {
	return a.Push(ADTNotice{Title: title, Description: description, Level: ADTWarning})
}

func (a *ADT) Error(title, description string) string {
	return a.Push(ADTNotice{Title: title, Description: description, Level: ADTError, Duration: adtDefaultDuration})
}

func (a *ADT) Action(title, description, action string, onAction func()) string {
	return a.Push(ADTNotice{
		Title: title, Description: description, Level: ADTAction,
		Action: action, OnAction: onAction,
	})
}

func (a *ADT) Loading(id, title, description string) string {
	return a.Push(ADTNotice{ID: id, Title: title, Description: description, Level: ADTLoading, Duration: -1})
}

// Render adds queued notices to MyGo's accessible toast lifecycle, then draws
// collapsed pills that spring open for details and close before they expire.
func (a *ADT) Render(c *ui.Context, theme *ui.Theme, config ADTConfig) {
	if a == nil {
		return
	}
	if config.MaxWidth <= 0 {
		config.MaxWidth = 350
	}
	if config.Top == 0 {
		config.Top = 62
	}
	if config.Right == 0 {
		config.Right = 18
	}
	if config.Bottom == 0 {
		config.Bottom = 18
	}
	if config.Left == 0 {
		config.Left = 18
	}
	for _, notice := range a.pending {
		if notice.Duration == 0 {
			notice.Duration = adtDefaultDuration
		}
		notice.createdAt = c.Now()
		c.AddToast(ui.Toast{
			ID: notice.ID, Title: notice.Title, Description: notice.Description,
			Type: string(notice.Level), Timeout: notice.Duration,
			Action: notice.Action, OnAction: notice.OnAction, Data: notice,
		})
	}
	clear(a.pending)
	a.pending = a.pending[:0]
	previousHoveredID := a.hoveredID
	a.hoveredID = ""

	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Left(config.Left).Right(config.Right).Top(config.Top).Bottom(config.Bottom).
			Width(config.MaxWidth).Gap(adtStackGap)
		if adtPositionIsBottom(config.Position) {
			viewport.Justify(ui.End)
		} else {
			viewport.Justify(ui.Start)
		}
		topMargin, rightMargin, bottomMargin, leftMargin := float32(0), float32(0), float32(0), float32(0)
		if adtPositionIsBottom(config.Position) {
			topMargin = ui.Auto
		}
		switch {
		case adtPositionIsLeft(config.Position):
			viewport.AlignItems(ui.Start)
			rightMargin = ui.Auto
		case adtPositionIsCenter(config.Position):
			viewport.AlignItems(ui.Center)
			rightMargin, leftMargin = ui.Auto, ui.Auto
		default:
			viewport.AlignItems(ui.End)
			leftMargin = ui.Auto
		}
		var stackHeight float32
		addHeight := func(height float32) {
			if stackHeight > 0 {
				stackHeight += adtStackGap
			}
			stackHeight += height
		}
		if adtPositionIsBottom(config.Position) {
			for i := range toasts {
				id := toasts[i].ID
				addHeight(adtToastCard(a, c, theme, config, toasts[i], i == len(toasts)-1, previousHoveredID == "" || previousHoveredID == id))
			}
		} else {
			for i := len(toasts) - 1; i >= 0; i-- {
				id := toasts[i].ID
				addHeight(adtToastCard(a, c, theme, config, toasts[i], i == len(toasts)-1, previousHoveredID == "" || previousHoveredID == id))
			}
		}
		viewport.Margin(topMargin, rightMargin, bottomMargin, leftMargin).
			Height(max(stackHeight, adtPillHeight))
		if previousHoveredID != "" && a.hoveredID == "" {
			c.After(time.Second / 60)
		}
	})
}

func adtToastCard(a *ADT, c *ui.Context, theme *ui.Theme, config ADTConfig, toast ui.Toast, active, canAutoExpand bool) float32 {
	notice, ok := toast.Data.(ADTNotice)
	if !ok {
		notice = ADTNotice{Level: ADTInfo, Duration: adtDefaultDuration, createdAt: c.Now()}
	}
	if notice.Duration == 0 {
		notice.Duration = adtDefaultDuration
	}
	if notice.ExpandAfter <= 0 {
		notice.ExpandAfter = adtExpandDelay
	}
	if notice.CollapseBefore <= 0 {
		notice.CollapseBefore = adtCollapseBuffer
	}
	if notice.Roundness <= 0 {
		notice.Roundness = 16
	}

	accent, icon := adtLevelStyle(theme, notice.Level)
	if notice.Icon != nil {
		icon = notice.Icon
	}
	fill, text := adtFill(theme)
	if notice.Fill != ui.Transparent {
		fill = notice.Fill
	}
	if notice.TextColor != ui.Transparent {
		text = notice.TextColor
	}

	parts := ui.ToastBase(c, toast)
	left := parts.Left()
	age := notice.Duration - left
	if notice.Duration < 0 {
		age = c.Now().Sub(notice.createdAt)
	}
	if age < 0 {
		age = 0
	}
	hovered := parts.Root.Hovered() || parts.Root.FocusWithin()
	if hovered && a != nil {
		a.hoveredID = toast.ID
	}
	loading := notice.Level == ADTLoading
	hasDetails := notice.Description != "" || notice.Action != ""
	collapseAt := notice.Duration - notice.CollapseBefore
	if notice.Duration > 0 && collapseAt < notice.ExpandAfter {
		collapseAt = notice.Duration
	}
	autoExpanded := active && canAutoExpand && !notice.ManualExpand && age >= notice.ExpandAfter &&
		(notice.Duration < 0 || age < collapseAt)
	expanded := hasDetails && !loading && (hovered || autoExpanded)
	if active && canAutoExpand && !hovered && hasDetails && !loading && !notice.ManualExpand {
		switch {
		case age < notice.ExpandAfter:
			c.After(notice.ExpandAfter - age)
		case notice.Duration > 0 && age < collapseAt:
			c.After(collapseAt - age)
		}
	}

	width := adtPillWidth(toast.Title, config.MaxWidth)
	if expanded {
		width = config.MaxWidth
	}
	cardHeight := adtPillHeight
	if expanded {
		cardHeight += adtExpandedBodyHeight(notice, width)
	}
	entryY := float32(-9)
	if adtPositionIsBottom(config.Position) {
		entryY = 9
	}
	root := parts.Root.Draggable(nil, ui.DraggableOptions{
		Axis: ui.Vertical, Animation: adtSpring,
		Snap: func(ui.Point) ui.Point { return ui.Point{} },
	}).Column().Width(width).MaxWidth(config.MaxWidth).MinWidth(adtPillWidth(toast.Title, config.MaxWidth)).
		Gap(0).Radius(notice.Roundness).Background(fill).
		Border(1, theme.Border.Alpha(0.18).Mix(accent, 0.25)).
		Shadow(0, 9, 24, 0, ui.RGBA(0, 0, 0, 0.23))
	root.Transition(ui.ElementTransition{
		Animation: adtTransition(), Position: true, Size: true, Colors: true,
		Enter: &ui.Motion{Y: entryY, Scale: 0.92, Opacity: 0, Collapse: true},
		Exit:  &ui.Motion{Y: entryY, Scale: 0.96, Opacity: 0, Collapse: true},
	})
	if _, velocityY, ended := root.DragEnded(); ended {
		offset := root.DragOffset()
		if math.Abs(float64(offset.Y)) > 32 || math.Abs(float64(velocityY)) > 900 {
			c.CloseToast(toast.ID)
		}
	}
	root.Children(func() {
		ui.Row(c).FillWidth().Height(40).AlignItems(ui.Center).Gap(8).Padding(7, 11).Children(func() {
			badge := ui.Row(c).Size(25, 25).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).
				Radius(13).Background(accent.Alpha(0.14)).TextColor(accent)
			badge.Children(func() {
				glyph := ui.Icon(c, icon).Size(15, 15).
					ContentTransition(ui.ContentTransition{Animation: contentAnimation()})
				if loading {
					turn := float32(c.Now().Sub(notice.createdAt).Seconds() * 360)
					c.AnimationFrame()
					glyph.AnimatePose(ui.Pose{Rotate: turn}, ui.Animation{Duration: 25 * time.Millisecond, Ease: ui.Linear})
				}
			})
			ui.Text(c, toast.Title).FontSize(12).FontWeight(500).TextColor(accent).
				FillWidth().SingleLine().ContentTransition(ui.ContentTransition{Animation: contentAnimation()})
			if expanded || hovered {
				close := parts.CloseButton().Size(24, 24).Shrink(0).Padding(0).Radius(12).
					TextColor(text.Alpha(0.62)).Label("Dismiss notification")
				if close.Hovered() {
					close.Background(accent.Alpha(0.14)).TextColor(accent)
				}
				close.Children(func() { ui.Text(c, "×").FontSize(16).TextColor(accent) })
			}
		})
		if expanded {
			ui.Column(c).FillWidth().Gap(9).Padding(1, 15, 13, 44).Children(func() {
				if toast.Description != "" {
					ui.Text(c, toast.Description).FontSize(12).TextColor(text.Alpha(0.68)).
						FillWidth().MaxHeight(80).ContentTransition(ui.ContentTransition{Animation: contentAnimation()})
				}
				if toast.Action != "" {
					button := parts.ActionButton().Padding(7, 10).Radius(10).
						Background(accent.Alpha(0.13)).TextColor(accent).Label(toast.Action)
					if button.Hovered() {
						button.Background(accent.Alpha(0.22))
					}
					button.Children(func() { ui.Text(c, toast.Action).FontSize(11).FontWeight(600) })
				}
			})
		}
	})
	return cardHeight
}

func adtPositionIsBottom(position ADTPosition) bool {
	return position == ADTBottomLeft || position == ADTBottomRight || position == ADTBottomCenter
}

func adtPositionIsLeft(position ADTPosition) bool {
	return position == ADTTopLeft || position == ADTBottomLeft
}

func adtPositionIsCenter(position ADTPosition) bool {
	return position == ADTTopCenter || position == ADTBottomCenter
}

func adtExpandedBodyHeight(notice ADTNotice, width float32) float32 {
	// Keep the invisible pause area close to the rendered toast so it does
	// not pause the queue while the pointer is elsewhere in the window.
	height := float32(14) // body padding
	if notice.Description != "" {
		contentWidth := max(width-59, 80)
		lineCount := max(1, int(math.Ceil(float64(utf8.RuneCountInString(notice.Description)*7)/float64(contentWidth))))
		descriptionHeight := min(float32(lineCount)*16+2, 80)
		height += descriptionHeight
	}
	if notice.Action != "" {
		if notice.Description != "" {
			height += adtStackGap
		}
		height += 28
	}
	return height
}

func adtPillWidth(title string, maxWidth float32) float32 {
	width := float32(utf8.RuneCountInString(title))*7 + 25 + 8 + 22
	return min(max(width, 142), maxWidth)
}

func adtFill(theme *ui.Theme) (ui.Color, ui.Color) {
	fill, text := theme.Inverse, theme.InverseText
	if fill == ui.Transparent {
		fill = theme.Text
	}
	if text == ui.Transparent {
		text = theme.Background
	}
	return fill, text
}

func adtTransition() *ui.Animation { return &adtSpring }

func contentAnimation() *ui.Animation {
	animation := ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut}
	return &animation
}

func adtLevelStyle(theme *ui.Theme, level ADTLevel) (ui.Color, *ui.SVG) {
	switch level {
	case ADTSuccess:
		return theme.Success, iconCheck
	case ADTWarning:
		return theme.Warning, iconAlert
	case ADTError:
		return theme.Danger, iconAlert
	case ADTLoading:
		return theme.Accent, adtLoader
	case ADTAction:
		return theme.Accent, iconPlay
	default:
		return theme.Accent, iconSparkles
	}
}
