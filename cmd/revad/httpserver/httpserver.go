package httpserver

import (
	"context"
	"net"
	"net/http"
	"sort"
	"time"

	"github.com/cernbox/reva/cmd/revad/svcs/httpsvcs"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"

	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus"
)

// NewMiddlewares contains all the registered new middleware functions.
var NewMiddlewares = map[string]NewMiddleware{}

// NewMiddleware is the function that HTTP middlewares need to register at init time.
type NewMiddleware func(conf map[string]interface{}) (Middleware, int, error)

// RegisterMiddleware registers a new HTTP middleware and its new function.
func RegisterMiddleware(name string, n NewMiddleware) {
	NewMiddlewares[name] = n
}

// middlewareTriple represents a middleware with the
// priority to be chained.
type middlewareTriple struct {
	Name       string
	Priority   int
	Middleware Middleware
}

// Middleware is a middleware http handler.
type Middleware func(h http.Handler) http.Handler

// Services is a map of service name and its new function.
var Services = map[string]NewService{}

// Register registers a new HTTP services with name and new function.
func Register(name string, newFunc NewService) {
	Services[name] = newFunc
}

// NewService is the function that HTTP services need to register at init time.
type NewService func(conf map[string]interface{}) (httpsvcs.Service, error)

var (
	ctx    = context.Background()
	logger = log.New("httpserver")
	errors = err.New("httpserver")
)

type config struct {
	Network            string                            `mapstructure:"network"`
	Address            string                            `mapstructure:"address"`
	Services           map[string]map[string]interface{} `mapstructure:"services"`
	EnabledServices    []string                          `mapstructure:"enabled_services"`
	Middlewares        map[string]map[string]interface{} `mapstructure:"middlewares"`
	EnabledMiddlewares []string                          `mapstructure:"enabled_middlewares"`
}

// Server contains the server info.
type Server struct {
	httpServer  *http.Server
	conf        *config
	listener    net.Listener
	svcs        map[string]http.Handler
	middlewares []*middlewareTriple
}

// New returns a new server
func New(m map[string]interface{}) (*Server, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	// apply defaults
	if conf.Network == "" {
		conf.Network = "tcp"
	}

	if conf.Address == "" {
		conf.Address = "0.0.0.0:9998"
	}

	httpServer := &http.Server{}
	s := &Server{
		httpServer: httpServer,
		conf:       conf,
		svcs:       map[string]http.Handler{},
	}
	return s, nil
}

// Start starts the server
func (s *Server) Start(ln net.Listener) error {
	if err := s.registerServices(); err != nil {
		return err
	}

	if err := s.registerMiddlewares(); err != nil {
		return err
	}

	s.httpServer.Handler = s.getHandler()
	s.listener = ln

	logger.Printf(ctx, "http server listening at %s:%s", s.conf.Network, s.conf.Address)
	err := s.httpServer.Serve(s.listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Stop stops the server.
func (s *Server) Stop() error {
	// TODO(labkode): set ctx deadline to zero
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// Network return the network type.
func (s *Server) Network() string {
	return s.conf.Network
}

// Address returns the network address.
func (s *Server) Address() string {
	return s.conf.Address
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) isEnabled(svcName string) bool {
	for _, key := range s.conf.EnabledServices {
		if key == svcName {
			return true
		}
	}
	return false
}

func (s *Server) isMiddlewareEnabled(name string) bool {
	for _, key := range s.conf.EnabledMiddlewares {
		if key == name {
			return true
		}
	}
	return false
}

func (s *Server) registerMiddlewares() error {
	middlewares := []*middlewareTriple{}
	for name, newFunc := range NewMiddlewares {
		if s.isMiddlewareEnabled(name) {
			m, prio, err := newFunc(s.conf.Middlewares[name])
			if err != nil {
				err = errors.Wrap(err, "error creating new middleware: "+name)
			}
			middlewares = append(middlewares, &middlewareTriple{
				Name:       name,
				Priority:   prio,
				Middleware: m,
			})
			logger.Printf(ctx, "http middleware enabled: %s", name)
		}
	}
	s.middlewares = middlewares
	return nil
}

func (s *Server) registerServices() error {
	svcs := map[string]http.Handler{}
	for svcName, newFunc := range Services {
		if s.isEnabled(svcName) {
			svc, err := newFunc(s.conf.Services[svcName])
			if err != nil {
				err = errors.Wrap(err, "error registering new http service")
				logger.Error(ctx, err)
				return err
			}
			svcs[svc.Prefix()] = prometheus.InstrumentHandler(svc.Prefix(), svc.Handler())
			logger.Printf(ctx, "http service enabled: %s@/%s", svcName, svc.Prefix())
		}
	}
	s.svcs = svcs
	return nil
}

func (s *Server) getHandler() http.Handler {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)
		if h, ok := s.svcs[head]; ok {
			logger.Println(r.Context(), "http routing: head=", head, " tail=", r.URL.Path, " svc="+head)
			h.ServeHTTP(w, r)
			return
		}
		logger.Println(r.Context(), "http routing: head=", head, " tail=", r.URL.Path, " svc=not-found")
		w.WriteHeader(http.StatusNotFound)
	})

	// sort middlewares by priority.
	sort.SliceStable(s.middlewares, func(i, j int) bool {
		return s.middlewares[i].Priority < s.middlewares[j].Priority
	})

	handler := http.Handler(h)
	for _, triple := range s.middlewares {
		logger.Printf(ctx, "chainning http middleware %s with priority  %d", triple.Name, triple.Priority)
		handler = triple.Middleware(handler)
	}
	return handler
}
