package prometheussvc

import (
	"fmt"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/httpsvr"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	httpsvr.Register("prometheussvc", New)
}

// New returns a new prometheus service
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	fmt.Println(conf)
	return &svc{prefix: conf.Prefix}, nil
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix  string
	handler http.Handler
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return promhttp.Handler()
}
