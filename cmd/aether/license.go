package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"aether-dots/internal/license"
)

const licenseUsage = "usage: aether license <status|activate|check|deactivate>\n  activate reads the key from stdin: echo \"$KEY\" | aether license activate"

func licenseCommand(dataDir string, args []string) error {
	return runLicense(dataDir, args, os.Stdin, os.Stdout, time.Now)
}

func runLicense(dataDir string, args []string, stdin io.Reader, out io.Writer, now func() time.Time) error {
	if len(args) != 1 {
		return errors.New(licenseUsage)
	}
	cfg, err := license.BuildConfig()
	if err != nil {
		return err
	}
	return runLicenseWith(cfg, license.NewClient(cfg), dataDir, args[0], stdin, out, now)
}

type licenseClient interface {
	Activate(context.Context, string) (*license.State, error)
	Validate(context.Context, *license.State) error
	Deactivate(context.Context, *license.State) error
}

func runLicenseWith(cfg license.Config, client licenseClient, dataDir, cmd string, stdin io.Reader, out io.Writer, now func() time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	state, err := license.Load(dataDir)
	if err != nil {
		return err
	}
	switch cmd {
	case "status":
		printEntitlement(out, cfg, state, now())
		return nil
	case "activate":
		if state != nil && !state.Revoked {
			return errors.New("a license is already active on this device; run `aether license deactivate` first")
		}
		line, err := bufio.NewReader(io.LimitReader(stdin, 1024)).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		next, err := client.Activate(ctx, line)
		if err != nil {
			return err
		}
		if err := license.Save(dataDir, next); err != nil {
			_ = client.Deactivate(ctx, next)
			return err
		}
		printEntitlement(out, cfg, next, now())
		return nil
	case "check":
		if state == nil {
			return errors.New("no license on this device; run `aether license activate`")
		}
		verr := client.Validate(ctx, state)
		if verr != nil && !errors.Is(verr, license.ErrNotFound) && !errors.Is(verr, license.ErrNotPermitted) {
			return verr // unreachable or mismatched: keep the saved state as it was
		}
		if err := license.Save(dataDir, state); err != nil {
			return err
		}
		printEntitlement(out, cfg, state, now())
		return verr
	case "deactivate":
		if state == nil {
			fmt.Fprintln(out, "No license on this device.")
			return nil
		}
		if err := client.Deactivate(ctx, state); err != nil && !errors.Is(err, license.ErrNotFound) {
			return fmt.Errorf("%w (the local license was kept; try again)", err)
		}
		if err := license.Remove(dataDir); err != nil {
			return err
		}
		fmt.Fprintln(out, "License removed from this device and its activation released.")
		return nil
	default:
		return errors.New(licenseUsage)
	}
}

func printEntitlement(out io.Writer, cfg license.Config, s *license.State, now time.Time) {
	e := license.Evaluate(s, cfg.BuildDate, now)
	fmt.Fprintf(out, "Tier: %s (%s)\n", e.Tier, e.Reason)
	if s == nil {
		return
	}
	if s.DisplayKey != "" {
		fmt.Fprintf(out, "Key: %s\n", s.DisplayKey)
	}
	if s.ExpiresAt != nil {
		fmt.Fprintf(out, "Paid through: %s\n", s.ExpiresAt.Format("2006-01-02"))
	}
	if !s.LastValidatedAt.IsZero() {
		fmt.Fprintf(out, "Last checked: %s\n", s.LastValidatedAt.Format(time.RFC3339))
	}
	if !e.Until.IsZero() {
		fmt.Fprintf(out, "Check again before: %s\n", e.Until.Format(time.RFC3339))
	}
}
