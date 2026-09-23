// Copyright 2018-2026 CERN
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

// Package gateway is the HTTP entry point to a reva deployment: one address
// that accepts every request and forwards it to the service that declared the
// route it matches.
//
// It keeps no route list of its own. Every HTTP service advertises the routes
// it declared in the service registry, and the gateway mirrors them into a
// router of the same kind the services themselves use, so a request is matched
// here exactly as it would be at the service. Adding an endpoint to a service,
// or a whole service to the deployment, needs no change here and none in
// whatever sits in front.
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sort"
	"sync/atomic"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

// mount is the whole URL space: the gateway is the entry point, so every path
// is its own until a mirrored route says which service owns it.
const mount = "/"

func init() {
	global.Register("gateway", New)
}

type config struct {
	// RefreshInterval is how often the mirrored routes are rebuilt from the
	// registry, picking up services that appeared or went away.
	RefreshInterval string `mapstructure:"refresh_interval"`
	// Services, when set, limits the gateway to these service names. Empty
	// means every HTTP service in the registry.
	Services []string `mapstructure:"services"`
	// Exclude drops these service names. Endpoints not meant for the outside
	// (pprof, prometheus) belong here.
	Exclude []string `mapstructure:"exclude"`
	// FlushInterval is how often a streamed response is flushed to the client.
	// Negative flushes immediately, which is what a long-lived response needs.
	FlushInterval string `mapstructure:"flush_interval"`
}

func (c *config) ApplyDefaults() {
	if c.RefreshInterval == "" {
		c.RefreshInterval = "10s"
	}
	if c.FlushInterval == "" {
		c.FlushInterval = "-1"
	}
}

type svc struct {
	conf    *config
	log     *zerolog.Logger
	refresh time.Duration

	// routes is the mirrored router, replaced wholesale on each refresh so a
	// request is never served from a half-built table.
	routes atomic.Pointer[router.Router]
	// mirrored is the route count last mirrored, to log only real changes.
	mirrored atomic.Int64

	stop chan struct{}
}

// New returns a new gateway service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	refresh, err := time.ParseDuration(c.RefreshInterval)
	if err != nil {
		return nil, fmt.Errorf("gateway: refresh_interval %q: %w", c.RefreshInterval, err)
	}

	s := &svc{
		conf:    &c,
		log:     appctx.GetLogger(ctx),
		refresh: refresh,
		stop:    make(chan struct{}),
	}
	s.routes.Store(router.New())

	// The registry holds nothing yet at construction time - this process
	// registers its own services only once every listener is bound - so the
	// first useful mirror is the one the refresh loop builds.
	go s.refreshLoop()

	return s, nil
}

func (s *svc) Close() error {
	close(s.stop)
	return nil
}

func (s *svc) Prefix() string { return mount }

// Routes claims the whole URL space. The gateway does not authenticate: it
// forwards the request as it arrived, and the service it forwards to applies
// its own auth to the route that was matched.
func (s *svc) Routes(r *router.Router) {
	r.Mount(mount, s, router.Unprotected())
}

func (s *svc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.routes.Load().ServeHTTP(w, r)
}

func (s *svc) refreshLoop() {
	t := time.NewTicker(s.refresh)
	defer t.Stop()

	s.rebuild()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.rebuild()
		}
	}
}

// rebuild mirrors the registry's HTTP routes into a fresh router and swaps it
// in.
//
// Declaring a route panics when two patterns overlap without one being more
// specific, and that can only show once several services are on the same
// router. A conflict therefore discards the half-built mirror and rebuilds
// without the service that caused it, so one bad service costs its own routes
// rather than everyone else's.
func (s *svc) rebuild() {
	candidates, ok := s.candidates()
	if !ok {
		return
	}

	skip := map[string]bool{}
	for range len(candidates) + 1 {
		mirror, count, bad := s.build(candidates, skip)
		if bad == "" {
			s.routes.Store(mirror)
			if prev := s.mirrored.Swap(int64(count)); prev != int64(count) {
				s.log.Info().Int("routes", count).Msg("gateway: mirrored routes from the registry")
			}
			return
		}
		s.log.Error().Str("service", bad).Msg("gateway: skipping service, its routes conflict with another's")
		skip[bad] = true
	}
}

// serviceRoutes is one service's advertised routes.
type serviceRoutes struct {
	name   string
	routes []router.Route
}

// candidates reads the routes of every HTTP service the gateway serves.
func (s *svc) candidates() ([]serviceRoutes, bool) {
	reg := service.GlobalRegistry()
	if reg == nil {
		return nil, false
	}
	services, err := reg.ListServices()
	if err != nil {
		s.log.Error().Err(err).Msg("gateway: listing services")
		return nil, false
	}

	out := make([]serviceRoutes, 0, len(services))
	for _, svc := range services {
		name := svc.Name()
		if !s.wants(name) {
			continue
		}
		if routes, ok := httpRoutes(svc); ok {
			out = append(out, serviceRoutes{name: name, routes: routes})
		}
	}
	// A stable order keeps which service loses a conflict from depending on
	// the registry's iteration order.
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, true
}

// build declares every candidate not skipped, returning the name of the first
// service whose routes the router refused.
func (s *svc) build(candidates []serviceRoutes, skip map[string]bool) (*router.Router, int, string) {
	mirror := router.New()
	var count int
	for _, c := range candidates {
		if skip[c.name] {
			continue
		}
		n, err := s.declare(mirror, c.name, c.routes)
		if err != nil {
			return nil, 0, c.name
		}
		count += n
	}
	return mirror, count, ""
}

// wants reports whether the gateway serves the named service. It never serves
// itself: mirroring its own catch-all would forward every request back here.
func (s *svc) wants(name string) bool {
	if name == "gateway" {
		return false
	}
	for _, e := range s.conf.Exclude {
		if e == name {
			return false
		}
	}
	if len(s.conf.Services) == 0 {
		return true
	}
	for _, w := range s.conf.Services {
		if w == name {
			return true
		}
	}
	return false
}

// declare puts the service's routes on the mirror, each forwarding to that
// service, turning the router's panic into an error for the caller.
func (s *svc) declare(mirror *router.Router, name string, routes []router.Route) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	proxy := s.proxyTo(name)
	view := mirror.Service(name)
	for _, rt := range routes {
		if rt.Subtree {
			view.Mount(rt.Pattern, proxy)
		} else {
			view.Handle(rt.Method, rt.Pattern, proxy)
		}
		n++
	}
	return n, nil
}

// httpRoutes returns the routes advertised by a service's nodes. Nodes of one
// service declare the same routes, so the first node that carries them wins.
func httpRoutes(svc registry.Service) ([]router.Route, bool) {
	for _, node := range svc.Nodes() {
		meta := node.Metadata()
		if meta[registry.MetaTransport] != registry.TransportHTTP {
			return nil, false
		}
		raw := meta[registry.MetaRoutes]
		if raw == "" {
			continue
		}
		var routes []router.Route
		if err := json.Unmarshal([]byte(raw), &routes); err != nil {
			continue
		}
		if len(routes) > 0 {
			return routes, true
		}
	}
	return nil, false
}

// proxyTo forwards a request to the named service, resolving a node per
// request so that a node going away, or a new one appearing, takes effect
// without waiting for the next refresh.
func (s *svc) proxyTo(name string) http.Handler {
	flush, err := time.ParseDuration(s.conf.FlushInterval)
	if err != nil {
		flush = -1
	}

	return &httputil.ReverseProxy{
		FlushInterval: flush,
		Rewrite: func(pr *httputil.ProxyRequest) {
			ep, err := service.HTTPEndpoint(pr.In.Context(), service.ByName(name))
			if err != nil {
				// Leave the outbound host empty: the transport fails, and
				// ErrorHandler turns it into a 502.
				return
			}
			// Only the authority is rewritten. The path is what the service
			// declared its routes against, and WebDAV in particular needs it
			// exactly as the client sent it, so Path and RawPath are untouched.
			pr.Out.URL.Scheme = ep.Scheme()
			pr.Out.URL.Host = ep.Address()
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			appctx.GetLogger(r.Context()).Error().Err(err).
				Str("service", name).Str("path", r.URL.Path).
				Msg("gateway: forwarding failed")
			w.WriteHeader(http.StatusBadGateway)
		},
	}
}
