// Copyright 2018-2024 CERN
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
	"net"
	"net/http"
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/activity"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

type Config func(*Server)

func WithServices(services map[string]global.Service) Config {
	return func(s *Server) {
		s.Services = services
	}
}

// WithRouter sets the router serving the server's services, as built by
// Routes.
func WithRouter(r *router.Router) Config {
	return func(s *Server) {
		s.router = r
	}
}

func WithMiddlewares(middlewares []global.Middleware) Config {
	return func(s *Server) {
		s.middlewares = middlewares
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

func InitServices(ctx context.Context, services map[string]config.ServicesConfig) (map[string]global.Service, error) {
	s := make(map[string]global.Service)
	for name, cfg := range services {
		new, ok := global.Services[name]
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

// Routes builds the router that serves the given services. Every service
// declares absolute patterns on it, so a request is matched once, against the
// whole server: there is no per-service prefix stripping, and two services
// claiming the same URL is a startup panic rather than a silent shadowing.
//
// counters, keyed by service name, are fed by the routes of that service.
func Routes(services map[string]global.Service, counters map[string]*activity.Counter, log *zerolog.Logger) *router.Router {
	root := router.New()
	for name, svc := range services {
		svc.Routes(root.Service(name).Use(serviceContext(name, counters[name])))
		log.Info().Msgf("http service enabled: %s@%s", name, svc.Prefix())
	}
	return root
}

// serviceContext stamps the owning service onto the request's context logger,
// so its logs are attributable, and records the request against the service's
// activity counter, feeding `admin services activity`.
func serviceContext(name string, counter *activity.Counter) router.Middleware {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if counter != nil {
				// HTTP has no per-method breakdown yet: count toward the
				// aggregate only.
				defer counter.Enter("")()
			}
			ctx := r.Context()
			log := appctx.GetLogger(ctx).With().Str("service", name).Logger()
			h.ServeHTTP(w, r.WithContext(appctx.WithLogger(ctx, &log)))
		})
	}
}

// New returns a new server.
func New(c ...Config) (*Server, error) {
	httpServer := &http.Server{}
	s := &Server{
		log:         zerolog.Nop(),
		httpServer:  httpServer,
		middlewares: []global.Middleware{},
	}
	for _, cc := range c {
		cc(s)
	}
	return s, nil
}

// Server contains the server info.
type Server struct {
	Services map[string]global.Service // map key is service name
	CertFile string
	KeyFile  string

	httpServer  *http.Server
	listener    net.Listener
	router      *router.Router
	middlewares []global.Middleware
	log         zerolog.Logger
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	s.httpServer.Handler = s.getHandler()
	s.listener = ln

	if (s.CertFile != "") && (s.KeyFile != "") {
		s.log.Info().Msgf("https server listening at https://%s using cert file '%s' and key file '%s'", s.listener.Addr(), s.CertFile, s.KeyFile)
		err := s.httpServer.ServeTLS(s.listener, s.CertFile, s.KeyFile)
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	}

	s.log.Info().Msgf("http server listening at http://%s", s.listener.Addr())
	err := s.httpServer.Serve(s.listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return err
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
	for name, svc := range s.Services {
		if err := svc.Close(); err != nil {
			s.log.Error().Err(err).Msgf("error closing service %q", name)
		} else {
			s.log.Info().Msgf("service %q correctly closed", name)
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

func (s *Server) getHandler() http.Handler {
	handler := http.Handler(s.router)
	for _, m := range s.middlewares {
		handler = m(handler)
	}
	return s.withRoute(handler)
}

// withRoute resolves the route a request will reach and puts it in the
// context, ahead of the middleware chain. It is what lets a middleware decide
// on the route itself - whether it is unprotected, who owns it - instead of
// matching the request path against a list kept somewhere else.
func (s *Server) withRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt, ok := s.router.Match(r); ok {
			r = r.WithContext(router.WithRoute(r.Context(), rt))
		}
		next.ServeHTTP(w, r)
	})
}
