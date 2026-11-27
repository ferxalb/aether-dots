// Package license checks a Polar license key for AETHER Pro.
//
// Licensing is off unless the build sets OrganizationID (see BuildConfig).
// A license only decides the Pro tier; it never gates access to local data.
//
// The rules, modelled on JetBrains' perpetual fallback license:
//   - Active: the key validated online within OfflineGrace and, for keys with
//     an expiry (annual licenses), the expiry has not passed.
//   - Fallback: a build released on or before the date the license was paid
//     through keeps Pro forever. Annual keys qualify at once; subscription keys
//     (no expiry in Polar) qualify after FallbackMinCoverage of validated use.
//   - Otherwise: Free.
package license

import (
	"errors"
	"strings"
	"time"
)

// Build-time settings, set with -ldflags "-X aether-dots/internal/license.OrganizationID=...".
var (
	// OrganizationID is the Polar organization UUID. Empty disables licensing.
	OrganizationID string
	// Environment is "production" (default) or "sandbox".
	Environment string
	// BuildDate is the RFC 3339 date this build was cut; used by the fallback rule.
	BuildDate string
)

const (
	productionBaseURL = "https://api.polar.sh"
	sandboxBaseURL    = "https://sandbox-api.polar.sh"

	// OfflineGrace is how long Pro stays active without a successful online check.
	OfflineGrace = 14 * 24 * time.Hour
	// FallbackMinCoverage is how long a subscription key must have been
	// validated before builds it covered keep Pro after it ends.
	FallbackMinCoverage = 365 * 24 * time.Hour

	maxKeyLength = 256
)

// Config is the resolved licensing configuration for this build.
type Config struct {
	OrganizationID string
	BaseURL        string
	BuildDate      time.Time
}

// ErrDisabled reports a build without licensing.
var ErrDisabled = errors.New("licensing is not enabled in this build")

// BuildConfig resolves the build-time settings. It returns ErrDisabled when
// OrganizationID is empty.
func BuildConfig() (Config, error) {
	org := strings.TrimSpace(OrganizationID)
	if org == "" {
		return Config{}, ErrDisabled
	}
	if !isUUID(org) {
		return Config{}, errors.New("license: OrganizationID is not a UUID")
	}
	cfg := Config{OrganizationID: org, BaseURL: productionBaseURL}
	switch strings.TrimSpace(Environment) {
	case "", "production":
	case "sandbox":
		cfg.BaseURL = sandboxBaseURL
	default:
		return Config{}, errors.New("license: Environment must be production or sandbox")
	}
	if BuildDate != "" {
		t, err := time.Parse(time.RFC3339, BuildDate)
		if err != nil {
			return Config{}, errors.New("license: BuildDate is not RFC 3339")
		}
		cfg.BuildDate = t.UTC()
	}
	return cfg, nil
}

// State is what AETHER remembers about the user's license.
type State struct {
	Version      int    `json:"version"`
	Key          string `json:"key"`
	DisplayKey   string `json:"display_key,omitempty"`
	ActivationID string `json:"activation_id"`
	// ActivatedAt is when this device activated the key.
	ActivatedAt time.Time `json:"activated_at"`
	// ExpiresAt is the key's expiry; nil for subscription keys.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// LastValidatedAt is the last successful online validation.
	LastValidatedAt time.Time `json:"last_validated_at"`
	// Revoked is set when Polar no longer recognises the key or activation.
	Revoked bool `json:"revoked,omitempty"`
}

const stateVersion = 1

// Tier is the product tier a license grants.
type Tier string

const (
	Free Tier = "free"
	Pro  Tier = "pro"
)

// Reason explains an Entitlement.
type Reason string

const (
	ReasonNoLicense   Reason = "no license"
	ReasonActive      Reason = "active"
	ReasonFallback    Reason = "fallback: this build was released while the license was paid"
	ReasonNeedsCheck  Reason = "not checked online recently"
	ReasonExpired     Reason = "expired"
	ReasonRevoked     Reason = "no longer valid"
	ReasonUnpaidBuild Reason = "this build is newer than the license"
)

// Entitlement is the result of Evaluate.
type Entitlement struct {
	Tier   Tier
	Reason Reason
	// Until is when an active entitlement next needs attention (expiry or the
	// end of the offline grace). Zero for fallback and Free.
	Until time.Time
}

// PaidThrough is the latest date the license is known to have been paid.
func (s *State) PaidThrough() time.Time {
	if s.ExpiresAt != nil && !s.Revoked {
		return s.ExpiresAt.UTC()
	}
	return s.LastValidatedAt.UTC()
}

func (s *State) fallbackEligible() bool {
	if s.ExpiresAt != nil {
		// A full term was bought up front. Revoked before its expiry means a
		// refund or a disabled key (a natural expiry is never marked revoked),
		// so it earns nothing.
		return !s.Revoked
	}
	return s.LastValidatedAt.Sub(s.ActivatedAt) >= FallbackMinCoverage
}

// Evaluate decides the tier for s at now, for a build cut on buildDate
// (zero for development builds, which get no fallback).
func Evaluate(s *State, buildDate, now time.Time) Entitlement {
	if s == nil || s.Key == "" {
		return Entitlement{Tier: Free, Reason: ReasonNoLicense}
	}
	graceEnd := s.LastValidatedAt.Add(OfflineGrace)
	expired := s.ExpiresAt != nil && !now.Before(*s.ExpiresAt)
	if !s.Revoked && !expired && now.Before(graceEnd) {
		until := graceEnd
		if s.ExpiresAt != nil && s.ExpiresAt.Before(until) {
			until = *s.ExpiresAt
		}
		return Entitlement{Tier: Pro, Reason: ReasonActive, Until: until.UTC()}
	}
	if !buildDate.IsZero() && s.fallbackEligible() && !buildDate.After(s.PaidThrough()) {
		return Entitlement{Tier: Pro, Reason: ReasonFallback}
	}
	switch {
	case s.Revoked:
		return Entitlement{Tier: Free, Reason: ReasonRevoked}
	case expired && !buildDate.IsZero() && s.fallbackEligible():
		return Entitlement{Tier: Free, Reason: ReasonUnpaidBuild}
	case expired:
		return Entitlement{Tier: Free, Reason: ReasonExpired}
	default:
		return Entitlement{Tier: Free, Reason: ReasonNeedsCheck}
	}
}

// NormalizeKey trims a pasted key and rejects anything that cannot be one.
func NormalizeKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > maxKeyLength {
		return "", errors.New("license key is empty or too long")
	}
	for _, r := range key {
		if r <= ' ' || r > '~' {
			return "", errors.New("license key contains unexpected characters")
		}
	}
	return key, nil
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range s {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}
