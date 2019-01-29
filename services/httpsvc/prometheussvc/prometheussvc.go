package prometheussvc

import (
	"net/http"

	"github.com/cernbox/reva/services/httpsvc"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix  string
	handler http.Handler
}

// New returns a new prometheus service
func New(m map[string]interface{}) (httpsvc.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	return &svc{prefix: conf.Prefix}, nil
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return promhttp.Handler()
}
