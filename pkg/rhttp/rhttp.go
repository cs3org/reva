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
	"sort"
	"strings"
	"time"

	"github.com/cs3org/reva/internal/http/interceptors/appctx"
	"github.com/cs3org/reva/internal/http/interceptors/auth"
	"github.com/cs3org/reva/internal/http/interceptors/log"
	"github.com/cs3org/reva/internal/http/interceptors/providerauthorizer"
	"github.com/cs3org/reva/pkg/rhttp/global"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/propagation"
)

// New returns a new server.
func New(m interface{}, l zerolog.Logger) (*Server, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	httpServer := &http.Server{}
	s := &Server{
		httpServer:  httpServer,
		conf:        conf,
		svcs:        map[string]global.Service{},
		unprotected: []string{},
		handlers:    map[string]http.Handler{},
		log:         l,
	}
	return s, nil
}

// Server contains the server info.
type Server struct {
	httpServer  *http.Server
	conf        *config
	listener    net.Listener
	svcs        map[string]global.Service // map key is svc Prefix
	unprotected []string
	handlers    map[string]http.Handler
	middlewares []*middlewareTriple
	log         zerolog.Logger
}

type config struct {
	Network     string                            `mapstructure:"network"`
	Address     string                            `mapstructure:"address"`
	Services    map[string]map[string]interface{} `mapstructure:"services"`
	Middlewares map[string]map[string]interface{} `mapstructure:"middlewares"`
	CertFile    string                            `mapstructure:"certfile"`
	KeyFile     string                            `mapstructure:"keyfile"`
}

func (c *config) init() {
	// apply defaults
	if c.Network == "" {
		c.Network = "tcp"
	}

	if c.Address == "" {
		c.Address = "0.0.0.0:19001"
	}
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	if err := s.registerServices(); err != nil {
		return err
	}

	if err := s.registerMiddlewares(); err != nil {
		return err
	}

	handler, err := s.getHandler()
	if err != nil {
		return errors.Wrap(err, "rhttp: error creating http handler")
	}

	s.httpServer.Handler = handler
	s.listener = ln

	if (s.conf.CertFile != "") && (s.conf.KeyFile != "") {
		s.log.Info().Msgf("https server listening at https://%s '%s' '%s'", s.conf.Address, s.conf.CertFile, s.conf.KeyFile)
		err = s.httpServer.ServeTLS(s.listener, s.conf.CertFile, s.conf.KeyFile)
	} else {
		s.log.Info().Msgf("http server listening at http://%s '%s' '%s'", s.conf.Address, s.conf.CertFile, s.conf.KeyFile)
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
	return s.conf.Network
}

// Address returns the network address.
func (s *Server) Address() string {
	return s.conf.Address
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() error {
	s.closeServices()
	return s.httpServer.Shutdown(context.Background())
}

// middlewareTriple represents a middleware with the
// priority to be chained.
type middlewareTriple struct {
	Name       string
	Priority   int
	Middleware global.Middleware
}

func (s *Server) registerMiddlewares() error {
	middlewares := []*middlewareTriple{}
	for name, newFunc := range global.NewMiddlewares {
		if s.isMiddlewareEnabled(name) {
			m, prio, err := newFunc(s.conf.Middlewares[name])
			if err != nil {
				err = errors.Wrapf(err, "error creating new middleware: %s,", name)
				return err
			}
			middlewares = append(middlewares, &middlewareTriple{
				Name:       name,
				Priority:   prio,
				Middleware: m,
			})
			s.log.Info().Msgf("http middleware enabled: %s", name)
		}
	}
	s.middlewares = middlewares
	return nil
}

func (s *Server) isMiddlewareEnabled(name string) bool {
	_, ok := s.conf.Middlewares[name]
	return ok
}

func (s *Server) registerServices() error {
	for svcName := range s.conf.Services {
		if s.isServiceEnabled(svcName) {
			newFunc := global.Services[svcName]
			svcLogger := s.log.With().Str("service", svcName).Logger()
			svc, err := newFunc(s.conf.Services[svcName], &svcLogger)
			if err != nil {
				err = errors.Wrapf(err, "http service %s could not be started,", svcName)
				return err
			}

			// instrument services with opencensus tracing.
			h := traceHandler(svcName, svc.Handler())
			s.handlers[svc.Prefix()] = h
			s.svcs[svc.Prefix()] = svc
			s.unprotected = append(s.unprotected, getUnprotected(svc.Prefix(), svc.Unprotected())...)
			s.log.Info().Msgf("http service enabled: %s@/%s", svcName, svc.Prefix())
		} else {
			message := fmt.Sprintf("http service %s does not exist", svcName)
			return errors.New(message)
		}
	}
	return nil
}

func (s *Server) isServiceEnabled(svcName string) bool {
	_, ok := global.Services[svcName]
	return ok
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

	// sort middlewares by priority.
	sort.SliceStable(s.middlewares, func(i, j int) bool {
		return s.middlewares[i].Priority > s.middlewares[j].Priority
	})

	handler := http.Handler(h)

	for _, triple := range s.middlewares {
		s.log.Info().Msgf("chaining http middleware %s with priority  %d", triple.Name, triple.Priority)
		handler = triple.Middleware(traceHandler(triple.Name, handler))
	}

	for _, v := range s.unprotected {
		s.log.Info().Msgf("unprotected URL: %s", v)
	}
	authMiddle, err := auth.New(s.conf.Middlewares["auth"], s.unprotected)
	if err != nil {
		return nil, errors.Wrap(err, "rhttp: error creating auth middleware")
	}

	// add always the logctx middleware as most priority, this middleware is internal
	// and cannot be configured from the configuration.
	coreMiddlewares := []*middlewareTriple{}

	providerAuthMiddle, err := addProviderAuthMiddleware(s.conf, s.unprotected)
	if err != nil {
		return nil, errors.Wrap(err, "rhttp: error creating providerauthorizer middleware")
	}
	if providerAuthMiddle != nil {
		coreMiddlewares = append(coreMiddlewares, &middlewareTriple{Middleware: providerAuthMiddle, Name: "providerauthorizer"})
	}

	coreMiddlewares = append(coreMiddlewares, &middlewareTriple{Middleware: authMiddle, Name: "auth"})
	coreMiddlewares = append(coreMiddlewares, &middlewareTriple{Middleware: log.New(), Name: "log"})
	coreMiddlewares = append(coreMiddlewares, &middlewareTriple{Middleware: appctx.New(s.log), Name: "appctx"})

	for _, triple := range coreMiddlewares {
		handler = triple.Middleware(traceHandler(triple.Name, handler))
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

func addProviderAuthMiddleware(conf *config, unprotected []string) (global.Middleware, error) {
	_, ocmdRegistered := global.Services["ocmd"]
	_, ocmdEnabled := conf.Services["ocmd"]
	ocmdPrefix, _ := conf.Services["ocmd"]["prefix"].(string)
	if ocmdRegistered && ocmdEnabled {
		return providerauthorizer.New(conf.Middlewares["providerauthorizer"], unprotected, ocmdPrefix)
	}
	return nil, nil
}
