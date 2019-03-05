package impersonator

import (
	"context"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
	"github.com/cernbox/reva/pkg/user"
)

func init() {
	registry.Register("impersonator", New)
}

type mgr struct{}

// New returns an auth manager implementation that allows to authenticate with any credentials.
func New(c map[string]interface{}) (auth.Manager, error) {
	return &mgr{}, nil
}

func (m *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) (*user.User, error) {
	return nil, nil
}
