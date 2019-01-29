package httpsvr

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cernbox/reva/services/httpsvc"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/services/httpsvc/handlers"
	"github.com/cernbox/reva/services/httpsvc/iframeuisvc"
	"github.com/cernbox/reva/services/httpsvc/ocdavsvc"
	"github.com/cernbox/reva/services/httpsvc/prometheussvc"
	"github.com/cernbox/reva/services/httpsvc/webuisvc"

	"github.com/mitchellh/mapstructure"
)

var (
	ctx    = context.Background()
	logger = log.New("httpsvr")
	errors = err.New("httpsvr")
)

type config struct {
	Network         string                 `mapstructure:"network"`
	Address         string                 `mapstructure:"address"`
	EnabledServices []string               `mapstructure:"enabled_services"`
	WebUISvc        map[string]interface{} `mapstructure:"webui_svc"`
	OCDAVSvc        map[string]interface{} `mapstructure:"ocdav_svc"`
	PromSvc         map[string]interface{} `mapstructure:"prometheus_svc"`
	IFrameUISvc     map[string]interface{} `mapstructure:"iframe_ui_svc"`
}

// Server contains the server info.
type Server struct {
	httpServer *http.Server
	conf       *config
	listener   net.Listener
	svcs       map[string]http.Handler
}

// New returns a new server
func New(m map[string]interface{}) (*Server, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	httpServer := &http.Server{}
	return &Server{httpServer: httpServer, conf: conf}, nil
}

// Start starts the server
func (s *Server) Start(ln net.Listener) error {
	if err := s.registerServices(); err != nil {
		return err
	}

	s.httpServer.Handler = s.getHandler()
	s.listener = ln
	err := s.httpServer.Serve(s.listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Stop() error {
	// TODO(labkode): set ctx deadline to zero
	ctx, _ = context.WithTimeout(ctx, time.Second)
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Network() string {
	return s.conf.Network
}

func (s *Server) Address() string {
	return s.conf.Address
}

func (s *Server) GracefulStop() error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) registerServices() error {
	svcs := map[string]http.Handler{}
	var svc httpsvc.Service
	var err error
	for _, k := range s.conf.EnabledServices {
		switch k {
		case "webui_svc":
			svc, err = webuisvc.New(s.conf.WebUISvc)
		case "ocdav_svc":
			svc, err = ocdavsvc.New(s.conf.OCDAVSvc)
		case "prometheus_svc":
			svc, err = prometheussvc.New(s.conf.PromSvc)
		case "iframe_ui_svc":
			svc, err = iframeuisvc.New(s.conf.IFrameUISvc)
		}

		if err != nil {
			return errors.Wrap(err, "unable to register service "+k)
		}
		svcs[svc.Prefix()] = svc.Handler()
	}

	if len(svcs) == 0 {
		logger.Println(ctx, "no services enabled")
	} else {
		for k := range s.conf.EnabledServices {
			logger.Printf(ctx, "http service enabled: %s", s.conf.EnabledServices[k])
		}
	}

	// instrument services with prometheus
	for prefix, h := range svcs {

		svcs[prefix] = prometheus.InstrumentHandler(prefix, h)
	}
	s.svcs = svcs
	return nil
}

func (s *Server) getHandler() http.Handler {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvc.ShiftPath(r.URL.Path)
		//logger.Println(r.Context(), "http routing: head=", head, " tail=", r.URL.Path)

		if h, ok := s.svcs[head]; ok {
			h.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	return handlers.TraceHandler(handlers.LogHandler(logger, h))
}
