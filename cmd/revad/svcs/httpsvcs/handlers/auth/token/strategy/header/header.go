package header

import (
	"context"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/token/registry"
	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

var logger = log.New("token-strategy-header")

func init() {
	registry.Register("header", New)
}

type config struct {
	Header string `mapstructure:"header"`
}
type strategy struct {
	header string
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
		return nil, err
	}
	logger.Println(context.Background(), "config: ", c)
	return c, nil
}

// New returns a new auth strategy that checks for basic auth.
func New(m map[string]interface{}) (auth.TokenStrategy, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &strategy{header: conf.Header}, nil
}

func (s *strategy) GetToken(r *http.Request) string {
	return r.Header.Get(s.header)
}
