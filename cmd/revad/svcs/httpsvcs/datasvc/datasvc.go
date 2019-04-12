package datasvc

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cernbox/reva/cmd/revad/httpserver"
	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("datasvc", New)
}

var logger = log.New("datasvc")

type config struct {
	Prefix       string                            `mapstructure:"prefix"`
	Driver       string                            `mapstructure:"driver"`
	TmpFolder    string                            `mapstructure:"tmp_folder"`
	Drivers      map[string]map[string]interface{} `mapstructure:"drivers"`
	ProviderPath string                            `mapstructure:"provider_path"`
}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
}

// New returns a new httpuploadsvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.TmpFolder == "" {
		conf.TmpFolder = os.TempDir()
	} else {
		os.MkdirAll(conf.TmpFolder, 0755)
	}

	fs, err := getFS(conf)
	if err != nil {
		return nil, err
	}

	s := &svc{
		storage: fs,
		conf:    conf,
	}
	s.setHandler()
	return s, nil
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			s.doGet(w, r)
			return
		case "PUT":
			s.doPut(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	})
}
