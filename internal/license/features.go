package license

import (
	"errors"
	"fmt"
	"time"
)

// FreeAgentLimit is how many agents a Free installation can create.
const FreeAgentLimit = 3

// ErrProRequired reports an action that needs AETHER Pro.
var ErrProRequired = errors.New("requires AETHER Pro")

// Features is what this build and the saved license allow.
//
// The Free/Pro split:
//   - Free: up to FreeAgentLimit agents, chat, GitHub read access, Midnight.
//   - Pro: unlimited agents, GitHub write proposals, Routines.
//
// Gates only stop creating more or starting paid work. Existing agents,
// conversations, routines and proposals stay readable, editable and
// deletable, and a pending proposal can still be approved or rejected.
type Features struct {
	// Licensing is false in builds without a Polar organization ID; such
	// builds (development, source builds) have every feature.
	Licensing bool
	Pro       bool
}

// CurrentFeatures resolves the features for the license saved in dataDir.
// An unreadable license file counts as Free; it never blocks local data.
func CurrentFeatures(dataDir string, now time.Time) Features {
	cfg, err := BuildConfig()
	if err != nil {
		return Features{Pro: true}
	}
	state, err := Load(dataDir)
	if err != nil {
		return Features{Licensing: true}
	}
	return FeaturesFor(cfg, state, now)
}

// FeaturesFor evaluates state for an enabled licensing configuration.
func FeaturesFor(cfg Config, state *State, now time.Time) Features {
	return Features{Licensing: true, Pro: Evaluate(state, cfg.BuildDate, now).Tier == Pro}
}

// CanCreateAgent reports whether one more agent may be created when
// existing agents already exist.
func (f Features) CanCreateAgent(existing int) error {
	if f.Pro || existing < FreeAgentLimit {
		return nil
	}
	return fmt.Errorf("Free includes %d agents; creating more %w", FreeAgentLimit, ErrProRequired)
}

// Require returns ErrProRequired, naming feature, unless Pro is available.
func (f Features) Require(feature string) error {
	if f.Pro {
		return nil
	}
	return fmt.Errorf("%s %w", feature, ErrProRequired)
}
