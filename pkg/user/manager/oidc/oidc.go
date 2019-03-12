package demo

import (
	"context"

	"github.com/cernbox/reva/pkg/auth/manager/oidc"
	"github.com/cernbox/reva/pkg/user"
	"github.com/cernbox/reva/pkg/user/manager/registry"
)

func init() {
	registry.Register("oidc", New)
}

type manager struct {
}

// New returns a new user manager.
func New(m map[string]interface{}) (user.Manager, error) {
	return &manager{}, nil
}

func (m *manager) GetUser(ctx context.Context, username string) (*user.User, error) {

	claims, ok := ctx.Value(oidc.ClaimsKey).(oidc.Claims)
	if !ok {
		return nil, userNotFoundError(username)
	}

	return &user.User{
		Subject:     claims.Subject, // a stable non reassignable id
		Issuer:      claims.Issuer,  // in the scope of this issuer
		Username:    claims.KCIdentity["kc.i.un"],
		Groups:      []string{},
		Mail:        claims.Email,
		DisplayName: claims.KCIdentity["kc.i.dn"],
	}, nil
}

func (m *manager) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	return []string{}, nil // FIXME implement GetUserGroups for oidc user manager
}

func (m *manager) IsInGroup(ctx context.Context, username, group string) (bool, error) {
	return false, nil // FIXME implement IsInGroup for oidc user manager
}

type userNotFoundError string

func (e userNotFoundError) Error() string { return string(e) }
