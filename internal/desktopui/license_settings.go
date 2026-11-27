package desktopui

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"aether-dots/internal/license"

	"github.com/egoist/mygo/ui"
)

// licenseRecheckAfter is how old the last successful check may be before the
// app checks again in the background at startup.
const licenseRecheckAfter = 24 * time.Hour

type licenseService interface {
	Activate(context.Context, string) (*license.State, error)
	Validate(context.Context, *license.State) error
	Deactivate(context.Context, *license.State) error
}

// licenseUI is the AETHER Pro card's state. It is inert unless the build
// enables licensing (license.BuildConfig).
type licenseUI struct {
	enabled  bool
	cfg      license.Config
	client   licenseService
	state    *license.State
	keyDraft string
	busy     string
	err      string
	request  uint64
	now      func() time.Time
}

func (a *app) initLicense() {
	cfg, err := license.BuildConfig()
	if err != nil {
		if !errors.Is(err, license.ErrDisabled) {
			log.Printf("licensing disabled: %v", err)
		}
		return
	}
	a.license = licenseUI{enabled: true, cfg: cfg, client: license.NewClient(cfg), now: time.Now}
	state, err := license.Load(a.dataDir)
	if err != nil {
		a.license.err = "The saved license couldn't be read: " + err.Error()
		return
	}
	a.license.state = state
}

func (a *app) licenseNow() time.Time {
	if a.license.now != nil {
		return a.license.now()
	}
	return time.Now()
}

// recheckLicenseIfStale validates a saved license in the background when its
// last successful check is older than licenseRecheckAfter. Only a definite
// answer from Polar changes the saved state; being offline is not an error.
func (a *app) recheckLicenseIfStale() {
	l := &a.license
	if !l.enabled || l.state == nil || l.busy != "" || a.licenseNow().Sub(l.state.LastValidatedAt) < licenseRecheckAfter {
		return
	}
	a.validateLicense(false)
}

func (a *app) activateLicense() {
	l := &a.license
	key := strings.TrimSpace(l.keyDraft)
	if !l.enabled || l.busy != "" || key == "" {
		return
	}
	client, dataDir := l.client, a.dataDir
	a.runLicenseTask("Activating…", func(ctx context.Context) func() {
		state, err := client.Activate(ctx, key)
		if err == nil {
			if err = license.Save(dataDir, state); err != nil {
				_ = client.Deactivate(ctx, state)
			}
		}
		return func() {
			if err != nil {
				a.license.err = licenseErrorText(err)
				return
			}
			a.license.state, a.license.keyDraft, a.license.err = state, "", ""
			a.adt.Success("AETHER Pro is active", "Thanks for supporting AETHER.")
		}
	})
}

func (a *app) validateLicense(userInitiated bool) {
	l := &a.license
	if !l.enabled || l.state == nil || l.busy != "" {
		return
	}
	next := *l.state
	client, dataDir := l.client, a.dataDir
	a.runLicenseTask("Checking…", func(ctx context.Context) func() {
		err := client.Validate(ctx, &next)
		definite := err == nil || errors.Is(err, license.ErrNotFound) || errors.Is(err, license.ErrNotPermitted)
		var saveErr error
		if definite {
			saveErr = license.Save(dataDir, &next)
		}
		return func() {
			switch {
			case saveErr != nil:
				a.license.err = "The license was checked but couldn't be saved: " + saveErr.Error()
			case definite:
				a.license.state = &next
				a.license.err = ""
				if err != nil {
					a.license.err = licenseErrorText(err)
				}
			case userInitiated:
				a.license.err = licenseErrorText(err)
			}
		}
	})
}

func (a *app) removeLicense() {
	l := &a.license
	if !l.enabled || l.state == nil || l.busy != "" {
		return
	}
	state := *l.state
	client, dataDir := l.client, a.dataDir
	a.runLicenseTask("Removing…", func(ctx context.Context) func() {
		err := client.Deactivate(ctx, &state)
		if err == nil || errors.Is(err, license.ErrNotFound) {
			err = license.Remove(dataDir)
		}
		return func() {
			if err != nil {
				a.license.err = licenseErrorText(err) + " The license stays on this Mac; try again."
				return
			}
			a.license.state, a.license.err = nil, ""
		}
	})
}

// runLicenseTask runs work off the UI thread when a window exists (inline in
// tests) and applies its result on the UI thread unless a newer task started.
func (a *app) runLicenseTask(label string, work func(context.Context) func()) {
	a.license.request++
	request := a.license.request
	a.license.busy, a.license.err = label, ""
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		apply := work(ctx)
		cancel()
		finish := func() {
			if request != a.license.request {
				return
			}
			a.license.busy = ""
			apply()
		}
		if a.win != nil {
			a.win.Update(finish)
		} else {
			finish()
		}
	}
	if a.win != nil {
		go run()
	} else {
		run()
	}
}

func licenseErrorText(err error) string {
	switch {
	case errors.Is(err, license.ErrNotPermitted):
		return "This key can't be used here: it may be revoked or expired, or already active on its maximum number of devices. Remove it from another device in the Polar customer portal, then try again."
	case errors.Is(err, license.ErrNotFound):
		return "Polar doesn't recognise this key or this device's activation."
	case errors.Is(err, license.ErrUnavailable):
		return "Couldn't reach the license service. Check your connection and try again."
	case errors.Is(err, license.ErrMismatch):
		return "The license service answered for a different product. Nothing was changed."
	default:
		return err.Error()
	}
}

func licenseBadge(e license.Entitlement) string {
	switch {
	case e.Tier == license.Pro && e.Reason == license.ReasonFallback:
		return "Pro · this version"
	case e.Tier == license.Pro:
		return "Pro"
	default:
		return "Free"
	}
}

func licenseExplanation(e license.Entitlement) string {
	switch e.Reason {
	case license.ReasonActive:
		return "Pro is active. AETHER checks the license about once a day and keeps working offline for up to 14 days."
	case license.ReasonFallback:
		return "Your license year has ended. This version keeps Pro for good; renew to get Pro in newer versions."
	case license.ReasonUnpaidBuild:
		return "This version came out after your license year ended. Renew to use Pro here, or keep using an earlier version."
	case license.ReasonExpired:
		return "Your license year has ended. Renew to use Pro again."
	case license.ReasonNeedsCheck:
		return "AETHER couldn't confirm the license for more than 14 days. Connect to the internet and check again."
	case license.ReasonRevoked:
		return "Polar no longer recognises this license. If you think this is a mistake, contact support with your receipt."
	default:
		return "Pro is an annual license. When the year ends, the version you have keeps Pro for good; newer versions need a renewal."
	}
}

func (a *app) licenseSettingsCard(c *ui.Context, theme *ui.Theme) {
	l := &a.license
	if !l.enabled {
		return
	}
	ent := license.Evaluate(l.state, l.cfg.BuildDate, a.licenseNow())
	busy := l.busy != ""
	card := ui.Column(c.Key("license-card")).FillWidth().MinWidth(0).Gap(14).Padding(17, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			mark := ui.Box(c).Size(42, 42).Shrink(0).Radius(14).
				Background(theme.Background).Border(1, theme.Border)
			mark.Center().Children(func() { ui.Icon(c, iconSparkles).Size(20, 20) })
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "AETHER Pro").FontSize(15).Bold()
				ui.Text(c, "Your agents, chats and data never depend on the license.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
			badge := licenseBadge(ent)
			color := theme.TextMuted
			if ent.Tier == license.Pro {
				color = theme.Success
			}
			if busy {
				badge, color = l.busy, theme.Accent
			}
			ui.Box(c).Shrink(0).Padding(6, 9).Radius(10).Background(color.Alpha(0.11)).TextColor(color).
				Children(func() { ui.Text(c, badge).FontSize(10).Bold() })
		})

		if l.err != "" {
			ui.Row(c).FillWidth().Gap(8).Padding(10, 11).Radius(10).
				Background(theme.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, l.err).FontSize(11).TextColor(theme.Danger).FillWidth()
			})
		}

		ui.Text(c, licenseExplanation(ent)).FontSize(12).FillWidth()

		if l.state == nil {
			ui.TextInput(c.Key("license-key"), &l.keyDraft).Placeholder("Paste your license key").
				Label("License key").Password().ReadOnly(busy).FillWidth()
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				if ui.PrimaryButton(c.Key("license-activate"), "Activate").Padding(8, 12).
					Disabled(busy || strings.TrimSpace(l.keyDraft) == "").Clicked() {
					a.activateLicense()
				}
			})
		} else {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				if l.state.DisplayKey != "" {
					ui.Text(c, "Key "+l.state.DisplayKey).FontSize(11).TextColor(theme.TextMuted)
				}
				if l.state.ExpiresAt != nil {
					ui.Text(c, "Paid through "+l.state.ExpiresAt.Format("January 2, 2006")).FontSize(11).TextColor(theme.TextMuted)
				}
				if !l.state.LastValidatedAt.IsZero() {
					ui.Text(c, "Last checked "+l.state.LastValidatedAt.Local().Format("January 2, 2006 at 15:04")).FontSize(11).TextColor(theme.TextMuted)
				}
			})
			ui.Divider(c)
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
				if ui.Button(c.Key("license-check"), "Check now").Padding(8, 12).Disabled(busy).Clicked() {
					a.validateLicense(true)
				}
				if ui.Button(c.Key("license-remove"), "Remove from this Mac").Padding(8, 12).Disabled(busy).Clicked() {
					a.removeLicense()
				}
			})
		}
		ui.Text(c, "Activation and checks send only the key, an activation ID and “"+license.DeviceLabel()+"” to Polar, the license seller.").
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
	})
}

// features is what the build and the license allow right now. Builds without
// licensing have everything.
func (a *app) features() license.Features {
	if !a.license.enabled {
		return license.Features{Pro: true}
	}
	return license.FeaturesFor(a.license.cfg, a.license.state, a.licenseNow())
}

// proRequired explains a blocked action and where to unlock it.
func (a *app) proRequired(title string, err error) {
	a.adt.Warning(title, err.Error()+". Add a license in Settings → General.")
}

// proNotice is a short inline note on a screen whose feature needs Pro.
func proNotice(c *ui.Context, theme *ui.Theme, key, text string) {
	ui.Row(c.Key(key)).FillWidth().Gap(8).Padding(10, 11).Radius(10).
		Background(theme.Accent.Alpha(0.08)).Children(func() {
		ui.Text(c, text).FontSize(11).FillWidth()
	})
}
