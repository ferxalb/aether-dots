package desktopui

import (
	"github.com/egoist/mygo/ui"
)

type dropdownOption struct {
	value string
	label string
}

// dropdownSelect is the shared look and behavior for single-value choices.
// Screens provide only the option values and labels.
func dropdownSelect(c *ui.Context, theme *ui.Theme, key, label string, selected *string, options []dropdownOption, disabled bool) ui.Element {
	selectOption := ui.SelectBase(c.Key(key), selected)
	trigger := selectOption.Trigger.FillWidth().MinWidth(0).Padding(8, 10).Gap(8).Radius(9).
		Background(theme.Background).Border(1, theme.Border).Label(label)
	if disabled {
		trigger.Disabled(true)
	}
	trigger.Children(func() {
		ui.Text(c, dropdownLabel(options, *selected)).FontSize(12).FillWidth().SingleLine()
		ui.Text(c, "⌄").FontSize(12).TextColor(theme.TextMuted)
	})
	selectOption.Popup(func(panel ui.Element) {
		panel.Padding(4).MaxHeight(312).Radius(10).Background(theme.Surface).Border(1, theme.Border).
			Shadow(0, 5, 16, 0, ui.RGBA(0, 0, 0, 0.22))
		list := ui.Scroll(c).FillWidth().MaxHeight(300).MinHeight(0)
		list.Children(func() {
			for _, option := range options {
				item := selectOption.Item(option.value).FillWidth().Padding(9, 10).Radius(7)
				if item.Highlighted() || option.value == *selected {
					item.Background(theme.Accent.Alpha(0.16)).TextColor(theme.Text)
				}
				item.Children(func() { ui.Text(c, option.label).FontSize(12).FillWidth().SingleLine() })
			}
		})
	})
	return trigger
}

func dropdownLabel(options []dropdownOption, selected string) string {
	for _, option := range options {
		if option.value == selected {
			return option.label
		}
	}
	return selected
}

func stringDropdownOptions(values []string, label func(string) string) []dropdownOption {
	options := make([]dropdownOption, len(values))
	for i, value := range values {
		optionLabel := value
		if label != nil {
			optionLabel = label(value)
		}
		options[i] = dropdownOption{value: value, label: optionLabel}
	}
	return options
}
