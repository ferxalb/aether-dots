package desktopui

import (
	"aether-dots/internal/task"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
)

func containsSpace(db task.Database, id string) bool {
	for _, space := range db.Spaces {
		if space.ID == id {
			return true
		}
	}
	return false
}

func findDot(db task.Database, id string) (task.Dot, bool) {
	for _, dot := range db.Dots {
		if dot.ID == id {
			return dot, true
		}
	}
	return task.Dot{}, false
}

func selectedSpaceName(db task.Database, id string) string {
	for _, space := range db.Spaces {
		if space.ID == id {
			return space.Name
		}
	}
	return "None"
}

func statusGlyph(status task.Status) string {
	switch status {
	case task.Done:
		return "✓"
	case task.Running:
		return "●"
	case task.Failed:
		return "!"
	case task.Paused:
		return "Ⅱ"
	case task.WaitingInput, task.WaitingApproval, task.WaitingDevice:
		return "◷"
	default:
		return "○"
	}
}

func humanStatus(status task.Status) string {
	return strings.ReplaceAll(string(status), "_", " ")
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return "…" + value[len(value)-max+1:]
}

func errorsIsCanceled(err error) bool {
	return err == context.Canceled
}

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx, dy := x-16, y-16
			if dx*dx+dy*dy <= 12*12 {
				img.Set(x, y, color.Black)
			}
		}
	}
	var b strings.Builder
	_ = png.Encode(writer{&b}, img)
	return []byte(b.String())
}

type writer struct{ b *strings.Builder }

func (w writer) Write(p []byte) (int, error) { return w.b.Write(p) }
