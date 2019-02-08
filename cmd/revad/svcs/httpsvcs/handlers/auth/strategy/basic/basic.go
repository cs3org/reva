package basic

import (
	"fmt"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/registry"

	"github.com/cernbox/reva/pkg/auth"
)

func init() {
	registry.Register("basic", New)
}

type strategy struct{}

// New returns a new auth strategy that checks for basic auth.
func New(m map[string]interface{}) (auth.Strategy, error) {
	return &strategy{}, nil
}

func (s *strategy) GetCredentials(r *http.Request) (*auth.Credentials, error) {
	id, secret, ok := r.BasicAuth()
	if !ok {
		return nil, fmt.Errorf("no basic auth provided")
	}
	return &auth.Credentials{ClientID: id, ClientSecret: secret}, nil
}
