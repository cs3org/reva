package helloworldsvc

import (
	"net/http"

	"github.com/cernbox/reva/cmd/revad/httpserver"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("helloworldsvc", New)
}

// New returns a new helloworld service
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	if conf.HelloMessage == "" {
		conf.HelloMessage = "Hello World!"
	}
	return &svc{conf: conf}, nil
}

type config struct {
	Prefix       string `mapstructure:"prefix"`
	HelloMessage string `mapstructure:"hello_message"`
}

type svc struct {
	handler http.Handler
	conf    *config
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(s.conf.HelloMessage))
	})
}
