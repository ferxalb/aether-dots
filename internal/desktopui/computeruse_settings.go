package desktopui

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) computerUseIntegrationCard(c *ui.Context, theme *ui.Theme) {
	status := a.computerUseStatus
	if a.computerUseLoading {
		status = "Installing"
	} else if a.computerUseRevoking {
		status = "Revoking access"
	} else if status == "" {
		status = "Not installed"
	}
	card := ui.Column(c.Key("integration-computer-use-card")).FillWidth().MinWidth(0).Gap(14).Padding(17, 18).
		Radius(15).Background(theme.Surface).Border(1, theme.Border)
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			mark := ui.Box(c).Size(42, 42).Shrink(0).Radius(14).
				Background(theme.Background).Border(1, theme.Border)
			mark.Center().Children(func() { ui.Text(c, "⌘").FontSize(20).Bold() })
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Text(c, "Computer Use").FontSize(15).Bold()
				ui.Text(c, "Let a Dot read the open Apple Notes window or control Apple Music playback.").
					FontSize(11).TextColor(theme.TextMuted).FillWidth()
			})
			computerUseStatusBadge(c, theme, status)
		})

		if a.computerUseError != "" {
			ui.Row(c).FillWidth().Gap(8).Padding(10, 11).Radius(10).
				Background(theme.Danger.Alpha(0.08)).Children(func() {
				ui.Text(c, a.computerUseError).FontSize(11).TextColor(theme.Danger).FillWidth()
			})
		}

		ui.Text(c, "Cua Driver installs separately in /Applications. AETHER uses the stable release and verifies its release signature. If CuaDriver is not listed under Accessibility, add /Applications/CuaDriver.app and switch it on. Screen Recording stays off.").
			FontSize(11).TextColor(theme.TextMuted).FillWidth()
		ui.Text(c, "AETHER disables Cua Driver's optional usage telemetry during setup.").
			FontSize(10).TextColor(theme.TextMuted).FillWidth()
		ui.Text(c, "Notes content exposed through the open window may be sent to the Dot's configured model provider. Review and grant each app scope in that Dot's profile.").
			FontSize(10).TextColor(theme.TextMuted).FillWidth()

		ui.Divider(c)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
			if a.computerUseLoading {
				ui.Text(c, "Installing and preparing the bounded Cua Driver session…").FontSize(11).TextColor(theme.TextMuted)
				if ui.Button(c.Key("computer-use-cancel-install"), "Cancel").Padding(8, 12).Clicked() {
					a.cancelComputerUseInstall()
				}
			} else if !a.computerUseInstalled {
				if ui.PrimaryButton(c.Key("computer-use-install"), "Install Cua Driver").Padding(8, 12).
					Disabled(a.computerUseRevoking || a.computerUseSettingsOpening).Clicked() {
					a.startComputerUseInstall()
				}
			} else {
				if ui.Button(c.Key("computer-use-accessibility"), "Open Accessibility settings").Padding(8, 12).
					Disabled(a.computerUseRevoking || a.computerUseSettingsOpening).Clicked() {
					a.openComputerUseAccessibilitySettings()
				}
				if ui.Button(c.Key("computer-use-update"), "Update Cua Driver").Padding(8, 12).
					Disabled(a.computerUseRevoking || a.computerUseSettingsOpening).Clicked() {
					a.startComputerUseInstall()
				}
			}
			if ui.Button(c.Key("computer-use-revoke"), "Revoke all Dot access").Padding(8, 12).
				Disabled(a.computerUseLoading || a.computerUseRevoking || a.computerUseSettingsOpening).Clicked() {
				a.revokeComputerUseAccess()
			}
		})
	})
}

func computerUseStatusBadge(c *ui.Context, theme *ui.Theme, status string) {
	color := theme.TextMuted
	switch status {
	case "Installed · bounded session ready":
		color = ui.Hex("#168459")
	case "Installing", "Revoking access":
		color = theme.Accent
	case "Setup failed":
		color = theme.Danger
	}
	ui.Box(c).Shrink(0).Padding(6, 9).Radius(10).Background(color.Alpha(0.11)).TextColor(color).
		Children(func() { ui.Text(c, status).FontSize(10).Bold() })
}

func (a *app) startComputerUseInstall() {
	if a.computerUseLoading || a.computerUseRevoking || a.computerUseService == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	wasInstalled := a.computerUseInstalled
	a.computerUseRequest++
	request := a.computerUseRequest
	a.computerUseInstallCancel = cancel
	a.computerUseLoading = true
	a.computerUseStatus = "Installing"
	a.computerUseError = ""
	win := a.win
	service := a.computerUseService
	go func() {
		installErr := service.Install(ctx)
		var permissionErr error
		if installErr == nil && !wasInstalled {
			permissionErr = service.OpenAccessibilitySettings()
		}
		var prepareErr error
		if installErr == nil {
			prepareCtx, prepareCancel := context.WithTimeout(ctx, 12*time.Second)
			prepareErr = service.Prepare(prepareCtx)
			prepareCancel()
		}
		canceled := errors.Is(ctx.Err(), context.Canceled)
		cancel()
		finish := func() {
			if request != a.computerUseRequest {
				return
			}
			a.computerUseInstallCancel = nil
			a.computerUseLoading = false
			a.computerUseInstalled = service.Installed()
			switch {
			case canceled:
				a.computerUseStatus = "Installation cancelled"
			case installErr != nil:
				a.computerUseStatus = "Setup failed"
				a.computerUseError = installErr.Error()
			case prepareErr != nil:
				a.computerUseStatus = "Installed · session will start on use"
				a.computerUseError = "Cua Driver installed, but the fast-start session could not be prepared. It will retry when a Dot uses an enabled scope. " + prepareErr.Error()
				if permissionErr != nil {
					a.computerUseError += " Accessibility settings could not be opened: " + permissionErr.Error()
				}
			case permissionErr != nil:
				a.computerUseStatus = "Installed · enable Accessibility access"
				a.computerUseError = "Cua Driver installed, but macOS Accessibility settings could not be opened. Use the button below to open them. " + permissionErr.Error()
			case !wasInstalled:
				a.computerUseStatus = "Installed · enable CuaDriver in Accessibility settings"
				a.computerUseError = "Add /Applications/CuaDriver.app if it is not listed, then switch it on. Screen Recording is not needed."
			default:
				a.computerUseStatus = "Installed · bounded session ready"
				a.computerUseError = ""
			}
			a.refresh()
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) cancelComputerUseInstall() {
	if a.computerUseInstallCancel != nil {
		a.computerUseRequest++
		a.computerUseInstallCancel()
		a.computerUseInstallCancel = nil
	}
	a.computerUseLoading = false
	a.computerUseStatus = "Installation cancelled"
	a.refresh()
}

func (a *app) openComputerUseAccessibilitySettings() {
	if a.computerUseService == nil || a.computerUseSettingsOpening || a.computerUseRevoking {
		return
	}
	a.computerUseSettingsOpening = true
	service := a.computerUseService
	win := a.win
	go func() {
		err := service.OpenAccessibilitySettings()
		finish := func() {
			a.computerUseSettingsOpening = false
			if err != nil {
				a.computerUseError = err.Error()
				a.adt.Error("Couldn't open Accessibility settings", err.Error())
			} else {
				a.computerUseError = ""
				a.computerUseStatus = "Installed · enable Cua Driver in Accessibility settings"
			}
			a.refresh()
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}

func (a *app) revokeComputerUseAccess() {
	if a.computerUseRevoking {
		return
	}
	a.computerUseRevoking = true
	a.computerUseStatus = "Revoking access"
	a.computerUseError = ""
	win := a.win
	database := a.db
	service := a.computerUseService
	go func() {
		err := database.RevokeAllComputerUse()
		if err == nil && service != nil {
			service.Stop()
		}
		finish := func() {
			a.computerUseRevoking = false
			if err != nil {
				a.computerUseStatus = "Access revoke failed"
				a.computerUseError = "AETHER couldn't confirm that all Dot grants were removed. Try again. " + err.Error()
				a.adt.Error("Computer Use access needs attention", a.computerUseError)
			} else if a.computerUseInstalled {
				a.computerUseStatus = "Installed · all Dot grants revoked"
				a.adt.Push(ADTNotice{Title: "Computer Use access revoked", Description: "AETHER removed all Dot scopes and stopped the bounded Cua Driver session.", Level: ADTInfo})
			} else {
				a.computerUseStatus = "All Dot grants revoked"
			}
			a.refresh()
		}
		if win != nil {
			win.Update(finish)
		} else {
			finish()
		}
	}()
}
