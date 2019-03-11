package header

import (
	"context"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/tokenwriter/registry"
	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/log"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

var logger = log.New("token-writer-header")

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

// New returns a new token writer strategy that stores token in a header.
func New(m map[string]interface{}) (auth.TokenWriter, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	return &strategy{header: conf.Header}, nil
}

func (s *strategy) WriteToken(token string, w http.ResponseWriter) {
	w.Header().Set(s.header, token)
}
