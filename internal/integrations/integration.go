// Package integrations describes the small, user-visible set of integrations
// AETHER can provide. Implementations live in separate provider packages.
package integrations

type Availability string

const (
	Available  Availability = "available"
	ComingSoon Availability = "coming_soon"
)

type Descriptor struct {
	ID           string
	Name         string
	Description  string
	Availability Availability
}

// InitialProviders is deliberately descriptive rather than a large generic
// connector interface: upcoming providers have different native consent and
// lifecycle requirements from the integrations already available.
func InitialProviders() []Descriptor {
	return []Descriptor{
		{ID: "github", Name: "GitHub", Description: "Let your Dots inspect repositories, issues, pull requests, and CI activity.", Availability: Available},
		{ID: "apple-reminders", Name: "Apple Reminders", Description: "Let your Dots work with selected reminders and lists.", Availability: ComingSoon},
	}
}
