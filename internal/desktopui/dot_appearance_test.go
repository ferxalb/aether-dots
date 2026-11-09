package desktopui

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDotAppearanceVariesDefaultsAndPreservesCustomGlyphs(t *testing.T) {
	firstIcon, firstColor := dotAppearance("first", defaultDotIcon, defaultDotColor)
	secondIcon, secondColor := dotAppearance("second", defaultDotIcon, defaultDotColor)
	if firstIcon == secondIcon && firstColor == secondColor {
		t.Fatal("two default Dots received the same appearance")
	}
	if icon, color := dotAppearance("first", "◈", "#1D69C8"); icon != "◈" || color != "#1D69C8" {
		t.Fatalf("custom appearance was changed to %q %q", icon, color)
	}
	if mobileDotIcon("dot:comet") != "▶" || mobileDotIcon("◈") != "◈" {
		t.Fatal("phone icon did not preserve the text representation")
	}
}

func TestDotAppearancePickerSavesAndResets(t *testing.T) {
	a := newMessengerTestApp(t)
	dot := createTestConversation(t, a, a.selectedSpaceID, "Inbox", "")
	a.selectConversation(dot.ID)
	a.openConversationProfile()
	tester := ui.NewTester(a.view, 1120, 760)
	for _, label := range []string{"Comet", "Apricot"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if a.profileIcon != "dot:comet" || a.profileColor != "#FFA347" || a.profileColorInput != "#FFA347" {
		t.Fatalf("avatar choices were not applied: %q %q %q", a.profileIcon, a.profileColor, a.profileColorInput)
	}
	if err := tester.Click("Custom avatar color"); err != nil {
		t.Fatal(err)
	}
	tester.Command("selectAll")
	tester.Type("#30B8CC")
	if err := tester.Click("Save profile"); err != nil {
		t.Fatal(err)
	}
	saved, _ := findConversation(a.db.Snapshot(), dot.ID)
	if saved.Icon != "dot:comet" || saved.Color != "#30B8CC" {
		t.Fatalf("custom avatar was not saved: %q %q", saved.Icon, saved.Color)
	}
	if err := tester.Click("Customize"); err != nil {
		t.Fatal(err)
	}
	if a.profileColorInput != "#30B8CC" {
		t.Fatalf("reopened custom color = %q", a.profileColorInput)
	}
	if err := tester.Click("Reset"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Save profile"); err != nil {
		t.Fatal(err)
	}
	saved, _ = findConversation(a.db.Snapshot(), dot.ID)
	if saved.Icon != defaultDotIcon || saved.Color != defaultDotColor {
		t.Fatalf("automatic avatar was not restored: %q %q", saved.Icon, saved.Color)
	}
}
