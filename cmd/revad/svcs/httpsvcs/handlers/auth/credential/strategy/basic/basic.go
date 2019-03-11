package basic

import (
	"fmt"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/registry"

	"github.com/cernbox/reva/pkg/auth"
)

func init() {
	registry.Register("basic", New)
}

type strategy struct{}

// New returns a new auth strategy that checks for basic auth.
func New(m map[string]interface{}) (auth.CredentialStrategy, error) {
	return &strategy{}, nil
}

func (s *strategy) GetCredentials(w http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	id, secret, ok := r.BasicAuth()
	if !ok {
		// TODO make realm configurable
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, r.Host))
		return nil, fmt.Errorf("no basic auth provided")
	}
	return &auth.Credentials{ClientID: id, ClientSecret: secret}, nil
}
