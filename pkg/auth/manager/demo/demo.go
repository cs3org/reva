package demo

import (
	"context"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	credentials map[string]string
}

// New returns a new auth Manager.
func New(m map[string]interface{}) (auth.Manager, error) {
	// m not used
	creds := getCredentials()
	return &manager{credentials: creds}, nil
}

func (m *manager) Authenticate(ctx context.Context, clientID, clientSecret string) error {
	if secret, ok := m.credentials[clientID]; ok {
		if secret == clientSecret {
			return nil
		}
	}
	return invalidCredentialsError(clientID)
}

func getCredentials() map[string]string {
	return map[string]string{
		"einstein": "relativity",
		"marie":    "radioactivity",
		"richard":  "superfluidity",
	}
}

type invalidCredentialsError string

func (e invalidCredentialsError) Error() string { return string(e) }
