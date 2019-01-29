package demo

import (
	"context"

	"github.com/cernbox/reva/pkg/auth"
)

type manager struct {
	credentials map[string]string
}

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
