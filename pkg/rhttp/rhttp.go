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
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/pkg/rhttp/global"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/propagation"
)

type Config func(*Server)

func WithServices(services map[string]global.Service) Config {
	return func(s *Server) {
		s.Services = services
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

func InitServices(services map[string]config.ServicesConfig, log *zerolog.Logger) (map[string]global.Service, error) {
	s := make(map[string]global.Service)
	for name, cfg := range services {
		new, ok := global.Services[name]
		if !ok {
			return nil, fmt.Errorf("http service %s does not exist", name)
		}
		if cfg.DriversNumber() > 1 {
			return nil, fmt.Errorf("service %s cannot have more than one driver in the same server", name)
		}
		log := log.With().Str("service", name).Logger()
		svc, err := new(cfg[0].Config, &log)
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
		log:         zerolog.Nop(),
		httpServer:  httpServer,
		svcs:        map[string]global.Service{},
		unprotected: []string{},
		handlers:    map[string]http.Handler{},
		middlewares: []global.Middleware{},
	}
	for _, cc := range c {
		cc(s)
	}
	s.registerServices()
	return s, nil
}

// Server contains the server info.
type Server struct {
	Services map[string]global.Service // map key is service name
	CertFile string
	KeyFile  string

	httpServer  *http.Server
	listener    net.Listener
	svcs        map[string]global.Service // map key is svc Prefix
	unprotected []string
	handlers    map[string]http.Handler
	middlewares []global.Middleware
	log         zerolog.Logger
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	handler, err := s.getHandler()
	if err != nil {
		return errors.Wrap(err, "rhttp: error creating http handler")
	}

	s.httpServer.Handler = handler
	s.listener = ln

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
			s.log.Error().Err(err).Msgf("error closing service %q", svc.Prefix())
		} else {
			s.log.Info().Msgf("service %q correctly closed", svc.Prefix())
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

func (s *Server) registerServices() {
	for name, svc := range s.Services {
		// instrument services with opencensus tracing.
		h := traceHandler(name, svc.Handler())
		s.handlers[svc.Prefix()] = h
		s.svcs[svc.Prefix()] = svc
		s.unprotected = append(s.unprotected, getUnprotected(svc.Prefix(), svc.Unprotected())...)
		s.log.Info().Msgf("http service enabled: %s@/%s", name, svc.Prefix())
	}
}

// TODO(labkode): if the http server is exposed under a basename we need to prepend
// to prefix.
func getUnprotected(prefix string, unprotected []string) []string {
	for i := range unprotected {
		unprotected[i] = path.Join("/", prefix, unprotected[i])
	}
	return unprotected
}

// clean the url putting a slash (/) at the beginning if it does not have it
// and removing the slashes at the end
// if the url is "/", the output is "".
func cleanURL(url string) string {
	if len(url) > 0 {
		if url[0] != '/' {
			url = "/" + url
		}
		url = strings.TrimRight(url, "/")
	}
	return url
}

func urlHasPrefix(url, prefix string) bool {
	url = cleanURL(url)
	prefix = cleanURL(prefix)

	partsURL := strings.Split(url, "/")
	partsPrefix := strings.Split(prefix, "/")

	if len(partsPrefix) > len(partsURL) {
		return false
	}

	for i, p := range partsPrefix {
		u := partsURL[i]
		if p != u {
			return false
		}
	}

	return true
}

func (s *Server) getHandlerLongestCommongURL(url string) (http.Handler, string, bool) {
	var match string

	for k := range s.handlers {
		if urlHasPrefix(url, k) && len(k) > len(match) {
			match = k
		}
	}

	h, ok := s.handlers[match]
	return h, match, ok
}

func getSubURL(url, prefix string) string {
	// pre cond: prefix is a prefix for url
	// example: url = "/api/v0/", prefix = "/api", res = "/v0"
	url = cleanURL(url)
	prefix = cleanURL(prefix)

	return url[len(prefix):]
}

func (s *Server) getHandler() (http.Handler, error) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := s.handlers[r.URL.Path]; ok {
			s.log.Debug().Msgf("http routing: url=%s", r.URL.Path)
			r.URL.Path = "/"
			h.ServeHTTP(w, r)
			return
		}

		// find by longest common path
		if h, url, ok := s.getHandlerLongestCommongURL(r.URL.Path); ok {
			s.log.Debug().Msgf("http routing: url=%s", url)
			r.URL.Path = getSubURL(r.URL.Path, url)
			h.ServeHTTP(w, r)
			return
		}

		s.log.Debug().Msgf("http routing: url=%s svc=not-found", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})

	handler := http.Handler(h)
	for _, m := range s.middlewares {
		handler = m(handler)
	}

	return handler, nil
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
