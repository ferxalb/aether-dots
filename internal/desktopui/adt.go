package desktopui

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	adtDefaultDuration = 6 * time.Second
	adtPillHeight      = float32(40)
	adtStackGap        = float32(9)
)

var (
	adtAnimation     = ui.Animation{Duration: 85 * time.Millisecond, Ease: ui.EaseOut}
	adtDragAnimation = ui.Animation{Duration: 100 * time.Millisecond, Ease: ui.EaseOut}
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

	Duration time.Duration // Zero uses the default; negative keeps it until dismissed.

	Action    string
	OnAction  func()
	Icon      *ui.SVG // Optional icon override.
	Fill      ui.Color
	TextColor ui.Color
	Roundness float32
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

// Render uses MyGo's accessible toast lifecycle and paints cards in parent,
// after scroll content, so they remain visible above the app's content.
// The newest toast stays expanded so its detail and action remain usable.
func (a *ADT) Render(c *ui.Context, theme *ui.Theme, config ADTConfig, parent ui.Element) {
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
				selected := previousHoveredID == id || (previousHoveredID == "" && i == len(toasts)-1)
				addHeight(adtToastCardHeight(c, config, toasts[i], selected))
			}
		} else {
			for i := len(toasts) - 1; i >= 0; i-- {
				id := toasts[i].ID
				selected := previousHoveredID == id || (previousHoveredID == "" && i == len(toasts)-1)
				addHeight(adtToastCardHeight(c, config, toasts[i], selected))
			}
		}
		viewport.Width(config.MaxWidth).Height(max(stackHeight, adtPillHeight))
		adtStackPosition(viewport, config, stackHeight)

		parent.Children(func() {
			stack := ui.Column(c).Key("adt-stack").Absolute().Width(config.MaxWidth).
				Height(max(stackHeight, adtPillHeight)).Gap(adtStackGap)
			adtStackPosition(stack, config, stackHeight)
			stack.Transition(ui.ElementTransition{Animation: adtTransition(), Position: true})
			stack.Children(func() {
				if adtPositionIsBottom(config.Position) {
					for i := range toasts {
						id := toasts[i].ID
						selected := previousHoveredID == id || (previousHoveredID == "" && i == len(toasts)-1)
						adtToastCard(a, c, theme, config, toasts[i], selected)
					}
					return
				}
				for i := len(toasts) - 1; i >= 0; i-- {
					id := toasts[i].ID
					selected := previousHoveredID == id || (previousHoveredID == "" && i == len(toasts)-1)
					adtToastCard(a, c, theme, config, toasts[i], selected)
				}
			})
		})
		if previousHoveredID != a.hoveredID {
			c.After(time.Second / 60)
		}
	})
}

func adtStackPosition(stack ui.Element, config ADTConfig, height float32) {
	stack.Left(config.Left).Right(config.Right).Top(config.Top).Bottom(config.Bottom)
	topMargin, rightMargin, bottomMargin, leftMargin := float32(0), float32(0), float32(0), float32(0)
	if adtPositionIsBottom(config.Position) {
		topMargin = ui.Auto
		stack.Justify(ui.End)
	} else {
		stack.Justify(ui.Start)
	}
	switch {
	case adtPositionIsLeft(config.Position):
		stack.AlignItems(ui.Start)
		rightMargin = ui.Auto
	case adtPositionIsCenter(config.Position):
		stack.AlignItems(ui.Center)
		rightMargin, leftMargin = ui.Auto, ui.Auto
	default:
		stack.AlignItems(ui.End)
		leftMargin = ui.Auto
	}
	stack.Margin(topMargin, rightMargin, bottomMargin, leftMargin).
		Height(max(height, adtPillHeight))
}

func adtToastCardHeight(c *ui.Context, config ADTConfig, toast ui.Toast, selected bool) float32 {
	notice, ok := toast.Data.(ADTNotice)
	if !ok {
		notice = ADTNotice{Level: ADTInfo, Duration: adtDefaultDuration}
	}
	if notice.Duration == 0 {
		notice.Duration = adtDefaultDuration
	}
	if !selected || (notice.Description == "" && notice.Action == "") {
		return adtPillHeight
	}
	return adtPillHeight + adtExpandedBodyHeight(c, notice, config.MaxWidth)
}

func adtToastCard(a *ADT, c *ui.Context, theme *ui.Theme, config ADTConfig, toast ui.Toast, selected bool) {
	notice, ok := toast.Data.(ADTNotice)
	if !ok {
		notice = ADTNotice{Level: ADTInfo, Duration: adtDefaultDuration}
	}
	if notice.Duration == 0 {
		notice.Duration = adtDefaultDuration
	}
	if notice.Roundness <= 0 {
		notice.Roundness = 16
	}

	accent, icon := adtLevelStyle(theme, notice.Level)
	if notice.Icon != nil {
		icon = notice.Icon
	}
	fill, text := adtFill(theme)
	titleText := theme.Text
	if titleText == ui.Transparent {
		titleText = text
	}
	mutedText := theme.TextMuted
	if mutedText == ui.Transparent {
		mutedText = text.Alpha(0.72)
	}
	if notice.Fill != ui.Transparent {
		fill = notice.Fill
	}
	if notice.TextColor != ui.Transparent {
		titleText = notice.TextColor
		mutedText = notice.TextColor.Alpha(0.72)
	}

	parts := ui.ToastBase(c, toast)
	hovered := parts.Root.Hovered() || parts.Root.FocusWithin()
	if hovered && a != nil {
		a.hoveredID = toast.ID
	}
	hasDetails := notice.Description != "" || notice.Action != ""
	expanded := hasDetails && selected

	closeVisible := expanded || hovered || notice.Duration < 0
	compactWidth := adtPillWidth(c, toast.Title, config.MaxWidth, closeVisible)
	width := compactWidth
	if expanded {
		width = config.MaxWidth
	}
	entryY := float32(-4)
	if adtPositionIsBottom(config.Position) {
		entryY = 4
	}
	root := parts.Root.Draggable(nil, ui.DraggableOptions{
		Axis: ui.Vertical, Animation: adtDragAnimation,
		Snap: func(ui.Point) ui.Point { return ui.Point{} },
	}).Column().Width(width).MaxWidth(config.MaxWidth).MinWidth(compactWidth).
		Gap(0).Radius(notice.Roundness).Background(fill).
		Border(1, theme.Border).
		Shadow(0, 9, 24, 0, ui.RGBA(0, 0, 0, adtShadowOpacity(theme)))
	root.Transition(ui.ElementTransition{
		Animation: adtTransition(), Position: true,
		Enter: &ui.Motion{Y: entryY, Scale: 0.985, Opacity: 0},
		Exit:  &ui.Motion{Y: entryY, Scale: 0.985, Opacity: 0},
	})
	if _, velocityY, ended := root.DragEnded(); ended {
		offset := root.DragOffset()
		if math.Abs(float64(offset.Y)) > 32 || math.Abs(float64(velocityY)) > 900 {
			c.CloseToast(toast.ID)
		}
	}
	buildHeader := func() {
		header := ui.Row(c).Key("adt-header-"+toast.ID).FillWidth().Height(40).
			AlignItems(ui.Center).Gap(8).Padding(7, 11)
		header.Children(func() {
			badge := ui.Row(c).Size(25, 25).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).
				Radius(13).Background(accent.Alpha(0.14)).TextColor(accent)
			badge.Children(func() {
				if notice.Level == ADTLoading && notice.Icon == nil {
					ui.Spinner(c).Size(15, 15).TextColor(accent).Label("Working")
				} else {
					ui.Icon(c, icon).Size(15, 15)
				}
			})
			ui.Text(c, toast.Title).FontSize(12).FontWeight(500).TextColor(titleText).
				FillWidth().SingleLine()
			if closeVisible {
				close := parts.CloseButton().Size(24, 24).Shrink(0).Padding(0).Radius(12).
					TextColor(mutedText).Label("Dismiss notification")
				closeColor := mutedText
				if close.Hovered() {
					close.Background(accent.Alpha(0.14)).TextColor(accent)
					closeColor = accent
				}
				close.Children(func() { ui.Text(c, "×").FontSize(16).TextColor(closeColor) })
			}
		})
	}
	buildBody := func() {
		if !expanded {
			return
		}
		bodyTop, bodyBottom := float32(1), float32(13)
		body := ui.Column(c).Key("adt-body-"+toast.ID).FillWidth().Gap(9).
			Padding(bodyTop, 15, bodyBottom, 44)
		body.Transition(ui.ElementTransition{
			Duration: 60 * time.Millisecond, Ease: ui.EaseOut,
			Enter: &ui.Motion{Opacity: 0},
			Exit:  &ui.Motion{Opacity: 0},
		})
		body.Children(func() {
			if toast.Description != "" {
				ui.Text(c, toast.Description).FontSize(12).TextColor(mutedText).
					FillWidth().MaxHeight(80)
			}
			if toast.Action != "" {
				button := parts.ActionButton().Padding(7, 10).Radius(10).
					Background(theme.Accent.Alpha(0.12)).TextColor(theme.Accent).Label(toast.Action)
				if button.Hovered() {
					button.Background(theme.Accent.Alpha(0.2))
				}
				button.Children(func() { ui.Text(c, toast.Action).FontSize(11).FontWeight(600) })
			}
		})
	}
	root.Children(func() {
		buildHeader()
		buildBody()
	})
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

func adtExpandedBodyHeight(c *ui.Context, notice ADTNotice, width float32) float32 {
	// Keep the invisible pause area close to the rendered toast so it does
	// not pause the queue while the pointer is elsewhere in the window.
	height := float32(14) // body padding
	if notice.Description != "" {
		contentWidth := max(width-59, 80)
		_, measuredHeight := c.MeasureText(contentWidth, ui.Span{Text: notice.Description, Size: 12})
		descriptionHeight := min(measuredHeight, 80)
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

func adtPillWidth(c *ui.Context, title string, maxWidth float32, closeVisible bool) float32 {
	textWidth, _ := c.MeasureText(0, ui.Span{Text: title, Size: 12, Weight: 500})
	width := textWidth + 25 + 8 + 22 // badge, gap and horizontal padding
	if closeVisible {
		width += 24 + 8 // dismiss button and its gap
	}
	return min(max(width, 142), maxWidth)
}

func adtFill(theme *ui.Theme) (ui.Color, ui.Color) {
	fill, text := theme.Surface, theme.Text
	if fill == ui.Transparent {
		fill = theme.Background
	}
	if text == ui.Transparent {
		text = theme.InverseText
	}
	if text == ui.Transparent {
		text = theme.Background
	}
	return fill, text
}

func adtShadowOpacity(theme *ui.Theme) float32 {
	if theme.Dark {
		return 0.28
	}
	return 0.12
}

func adtTransition() *ui.Animation { return &adtAnimation }

func adtLevelStyle(theme *ui.Theme, level ADTLevel) (ui.Color, *ui.SVG) {
	switch level {
	case ADTSuccess:
		return theme.Success, iconCheck
	case ADTWarning:
		return theme.Warning, iconAlert
	case ADTError:
		return theme.Danger, iconAlert
	case ADTLoading:
		return theme.Accent, nil
	case ADTAction:
		return theme.Accent, iconPlay
	default:
		return theme.Accent, iconSparkles
	}
}
