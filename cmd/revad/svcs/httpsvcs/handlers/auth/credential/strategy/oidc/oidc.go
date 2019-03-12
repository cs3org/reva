package oidc

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/registry"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/log"
)

var logger = log.New("auth-strategy-oidc")

func init() {
	registry.Register("oidc", New)
}

type strategy struct{}

// New returns a new auth strategy that checks for oidc auth.
func New(m map[string]interface{}) (auth.CredentialStrategy, error) {
	return &strategy{}, nil
}

func (s *strategy) GetCredentials(w http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	// for time being just use OpenConnectID Connect
	hdr := r.Header.Get("Authorization")
	token := strings.TrimPrefix(hdr, "Bearer ")
	if token == "" {
		// TODO make realm configurable
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, r.Host))
		return nil, fmt.Errorf("no Bearer auth provided")
	}

	return &auth.Credentials{ClientSecret: token}, nil
}
