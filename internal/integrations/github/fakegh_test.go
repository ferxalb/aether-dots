package github

import (
	"context"
	"testing"

	"aether-dots/internal/integrations/github/ghtest"
)

type route = ghtest.Route

// newFakeGH returns a Service wired to a fake gh and the fake's call log.
func newFakeGH(t *testing.T, signedIn bool, routes ...ghtest.Route) (*Service, string) {
	t.Helper()
	binary, log := ghtest.Script(t, signedIn, routes...)
	return &Service{gh: &ghRunner{path: binary}, operations: map[uint64]context.CancelFunc{}}, log
}
