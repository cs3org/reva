// Copyright 2018-2023 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package rhttp

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/propagation"
)

// NewMiddlewares contains all the registered new middleware functions.
var NewMiddlewares = map[string]NewMiddleware{}

// NewMiddleware is the function that HTTP middlewares need to register at init time.
type NewMiddleware func(conf map[string]interface{}) (Middleware, int, error)

// RegisterMiddleware registers a new HTTP middleware and its new function.
func RegisterMiddleware(name string, n NewMiddleware) {
	NewMiddlewares[name] = n
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
type NewService func(context.Context, map[string]interface{}) (Service, error)

// Service represents a HTTP service.
type Service interface {
	Name() string
	Register(r mux.Router)
	io.Closer
}

type Config func(*Server)

func WithServices(services map[string]Service) Config {
	return func(s *Server) {
		s.svcs = services
	}
}

func WithCertAndKeyFiles(cert, key string) Config {
	return func(s *Server) {
		s.CertFile = cert
		s.KeyFile = key
	}
}

func WithLogger(log zerolog.Logger) Config {
	return func(s *Server) {
		s.log = log
	}
}

func WithMiddlewareFactory(f func(o *mux.Options) []mux.Middleware) Config {
	return func(s *Server) {
		s.midFactory = f
	}
}

func InitServices(ctx context.Context, services map[string]config.ServicesConfig) (map[string]Service, error) {
	s := make(map[string]Service)
	for name, cfg := range services {
		new, ok := Services[name]
		if !ok {
			return nil, fmt.Errorf("http service %s does not exist", name)
		}
		if cfg.DriversNumber() > 1 {
			return nil, fmt.Errorf("service %s cannot have more than one driver in the same server", name)
		}
		log := appctx.GetLogger(ctx).With().Str("service", name).Logger()
		ctx := appctx.WithLogger(ctx, &log)
		svc, err := new(ctx, cfg[0].Config)
		if err != nil {
			return nil, errors.Wrapf(err, "http service %s could not be started", name)
		}
		s[name] = svc
	}
	return s, nil
}

// New returns a new server.
func New(c ...Config) (*Server, error) {
	httpServer := &http.Server{}
	s := &Server{
		log:        zerolog.Nop(),
		httpServer: httpServer,
		svcs:       map[string]Service{},
	}
	for _, cc := range c {
		cc(s)
	}
	return s, nil
}

// Server contains the server info.
type Server struct {
	CertFile string
	KeyFile  string

	httpServer *http.Server
	listener   net.Listener
	svcs       map[string]Service // map key is svc Prefix
	log        zerolog.Logger
	midFactory func(*mux.Options) []mux.Middleware
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	router := mux.NewServeMux()
	router.SetMiddlewaresFactory(s.midFactory)

	s.registerServices(router)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router.Walk(ctx, func(method, path string, handler http.Handler, opts *mux.Options) {
		str := fmt.Sprintf("%s\t%s", method, path)
		if o := opts.String(); o != "" {
			str += fmt.Sprintf(" (%s)", o)
		}
		s.log.Debug().Msg(str)
	})

	s.httpServer.Handler = router
	s.listener = ln

	var err error
	if (s.CertFile != "") && (s.KeyFile != "") {
		s.log.Info().Msgf("https server listening at https://%s using cert file '%s' and key file '%s'", s.listener.Addr(), s.CertFile, s.KeyFile)
		err = s.httpServer.ServeTLS(s.listener, s.CertFile, s.KeyFile)
	} else {
		s.log.Info().Msgf("http server listening at http://%s", s.listener.Addr())
		err = s.httpServer.Serve(s.listener)
	}
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) registerServices(r mux.Router) {
	for _, svc := range s.svcs {
		svc.Register(r)
		s.log.Info().Msgf("http service enabled: %s", svc.Name())
	}
}

// Stop stops the server.
func (s *Server) Stop() error {
	s.closeServices()
	// TODO(labkode): set ctx deadline to zero
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}

// TODO(labkode): we can't stop the server shutdown because a service cannot be shutdown.
// What do we do in case a service cannot be properly closed? Now we just log the error.
// TODO(labkode): the close should be given a deadline using context.Context.
func (s *Server) closeServices() {
	for _, svc := range s.svcs {
		if err := svc.Close(); err != nil {
			s.log.Error().Err(err).Msgf("error closing service %s", svc.Name())
		} else {
			s.log.Info().Msgf("service %s correctly closed", svc.Name())
		}
	}
}

// Network return the network type.
func (s *Server) Network() string {
	return s.listener.Addr().Network()
}

// Address returns the network address.
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() error {
	s.closeServices()
	return s.httpServer.Shutdown(context.Background())
}

func traceHandler(name string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := rtrace.Propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		t := rtrace.Provider.Tracer("reva")
		ctx, span := t.Start(ctx, name)
		defer span.End()

		rtrace.Propagator.Inject(ctx, propagation.HeaderCarrier(r.Header))
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}
