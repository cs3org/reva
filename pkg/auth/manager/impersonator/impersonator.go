package impersonator

import (
	"context"

	"github.com/cernbox/reva/pkg/auth"
)

type mgr struct{}

// New returns an auth manager implementation that allows to authenticate with any credentials.
func New() auth.Manager {
	return &mgr{}
}

func (m *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) error {
	return nil
}
