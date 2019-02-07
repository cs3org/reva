package webuisvc

import (
	"net/http"

	"github.com/cernbox/reva/cmd/revad/httpsvr"
	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpsvr.Register("webuisvc", New)
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix  string
	handler http.Handler
}

// New returns a new webuisvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	return &svc{prefix: conf.Prefix, handler: getHandler()}, nil
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func getHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		html := `
		<!DOCTYPE html>
		<html>
		<body>
		
		<h1>Your favourite sync and share web UI will go here</h1>
		
		</body>
		</html>
		`
		w.Write([]byte(html))
	})
}
