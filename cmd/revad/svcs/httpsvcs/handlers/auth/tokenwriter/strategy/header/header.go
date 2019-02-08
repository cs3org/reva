package header

import (
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/tokenwriter/registry"
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

// New returns a new token writer strategy that stores token in a header.
func New(m map[string]interface{}) (auth.TokenWriter, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	return &strategy{header: conf.Header}, nil
}

func (s *strategy) WriteToken(token string, w http.ResponseWriter) {
	w.Header().Set(s.header, token)
}
