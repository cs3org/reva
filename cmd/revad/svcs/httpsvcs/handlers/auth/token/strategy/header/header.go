package header

import (
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/token/registry"
	"github.com/cernbox/reva/pkg/auth"
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("header", New)
}

type config struct {
	Header string `mapstructure:"header"`
}
type strategy struct {
	header string
}

// New returns a new auth strategy that checks for basic auth.
func New(m map[string]interface{}) (auth.TokenStrategy, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	return &strategy{header: conf.Header}, nil
}

func (s *strategy) GetToken(r *http.Request) string {
	return r.Header.Get(s.header)
}
