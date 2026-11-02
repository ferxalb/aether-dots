package desktopui

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// ADTLevel gives a notification its semantic color and icon.
type ADTLevel string

const (
	ADTInfo    ADTLevel = "info"
	ADTSuccess ADTLevel = "success"
	ADTWarning ADTLevel = "warning"
	ADTError   ADTLevel = "error"
)

// ADTNotice is one transient notification. An empty ID gives it a unique
// identity; reusing an ID updates the visible notice and restarts its timer.
type ADTNotice struct {
	ID          string
	Title       string
	Description string
	Level       ADTLevel
	Duration    time.Duration
	Action      string
	OnAction    func()
}

// ADTConfig places the toast viewport within the window. Insets leave room for
// persistent UI such as the sidebar and title bar.
type ADTConfig struct {
	Position ADTPosition
	MaxWidth float32
	Top      float32
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
)

// ADT queues notifications and renders them as a theme-aware native stack.
// Keep it in app state; never store a UI Context or Element here.
type ADT struct {
	pending []ADTNotice
	nextID  uint64
}

// Push queues a notice for the next view pass. It is safe to call from a
// Window.Update callback, which runs on the UI thread.
func (a *ADT) Push(notice ADTNotice) string {
	if a == nil || strings.TrimSpace(notice.Title) == "" {
		return ""
	}
	if notice.Level == "" {
		notice.Level = ADTInfo
	}
	if notice.ID == "" {
		a.nextID++
		notice.ID = fmt.Sprintf("aether-adt-%d", a.nextID)
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
	return a.Push(ADTNotice{Title: title, Description: description, Level: ADTError, Duration: 6 * time.Second})
}

// Render adds queued notices to MyGo's accessible toast lifecycle and draws the
// ADT cards. Colors are resolved from the current app theme on every frame.
func (a *ADT) Render(c *ui.Context, theme *ui.Theme, config ADTConfig) {
	if a == nil {
		return
	}
	if config.MaxWidth <= 0 {
		config.MaxWidth = 420
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
		c.AddToast(ui.Toast{
			ID: notice.ID, Title: notice.Title, Description: notice.Description,
			Type: string(notice.Level), Timeout: notice.Duration,
			Action: notice.Action, OnAction: notice.OnAction, Data: notice,
		})
	}
	clear(a.pending)
	a.pending = a.pending[:0]

	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Left(config.Left).Right(config.Right).Top(config.Top).Bottom(config.Bottom).
			AlignItems(ui.End).Gap(9)
		if config.Position == ADTBottomLeft || config.Position == ADTBottomRight {
			viewport.Justify(ui.End)
		} else {
			viewport.Justify(ui.Start)
		}
		if config.Position == ADTTopLeft || config.Position == ADTBottomLeft {
			viewport.AlignItems(ui.Start)
		}
		for i := len(toasts) - 1; i >= 0; i-- {
			adtToastCard(c, theme, config, toasts[i])
		}
	})
}

func adtToastCard(c *ui.Context, theme *ui.Theme, config ADTConfig, notice ui.Toast) {
	data, ok := notice.Data.(ADTNotice)
	if !ok {
		data = ADTNotice{Level: ADTInfo}
	}
	accent, icon := adtLevelStyle(theme, data.Level)
	parts := ui.ToastBase(c, notice)
	card := parts.Root.Column().FillWidth().MaxWidth(config.MaxWidth).Shrink(0).
		Gap(0).Radius(15).Background(theme.Surface).
		Border(1, theme.Border.Mix(accent, 0.3)).
		Shadow(0, 10, 26, 0, ui.RGBA(0, 0, 0, 0.24))
	card.Transition(ui.ElementTransition{
		Animation: &ui.Snappy,
		Enter:     &ui.Motion{Y: -14, Scale: 0.96, Opacity: 0},
		Exit:      &ui.Motion{Y: -8, Scale: 0.97, Opacity: 0},
	})
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(11).Padding(13, 14).Children(func() {
			ui.Box(c).Size(36, 36).Shrink(0).Radius(12).
				Background(accent.Alpha(0.14)).TextColor(accent).Children(func() {
				ui.Icon(c, icon).Size(17, 17)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Padding(1, 0).Children(func() {
				ui.Text(c, notice.Title).FontSize(12).Bold().TextColor(theme.Text).FillWidth()
				if notice.Description != "" {
					ui.Text(c, notice.Description).FontSize(11).TextColor(theme.TextMuted).
						FillWidth().MaxHeight(72)
				}
			})
			if notice.Action != "" {
				button := parts.ActionButton().Padding(6, 9).Radius(8).
					Background(accent.Alpha(0.12)).TextColor(accent).Label(notice.Action)
				button.Children(func() { ui.Text(c, notice.Action).FontSize(10).Bold() })
			}
			close := parts.CloseButton().Padding(3, 7).Radius(8).
				TextColor(theme.TextMuted).Label("Dismiss notification")
			if close.Hovered() {
				close.Background(theme.SurfaceHover)
			}
			close.Children(func() { ui.Text(c, "×").FontSize(16).TextColor(theme.TextMuted) })
		})
	})
}

func adtLevelStyle(theme *ui.Theme, level ADTLevel) (ui.Color, *ui.SVG) {
	switch level {
	case ADTSuccess:
		return theme.Success, iconCheck
	case ADTWarning:
		return theme.Warning, iconAlert
	case ADTError:
		return theme.Danger, iconAlert
	default:
		return theme.Accent, iconSparkles
	}
}
