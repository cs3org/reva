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

package wellknown

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/ratelimit"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("wellknown", New)
}

type svc struct {
	router  chi.Router
	Conf    *config
	limiter *ratelimit.Limiter
}

type config struct {
	OCMProvider OcmProviderConfig `mapstructure:"ocmprovider"`
	// RateLimit guards the (unauthenticated) discovery endpoint.
	RateLimit ratelimit.ServiceConfig `mapstructure:",squash"`
}

func (c *config) ApplyDefaults() {
	c.RateLimit.ApplyDefaults()
}

// New returns a new wellknown object.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	s := &svc{
		router: r,
		Conf:   &c,
	}
	if err := s.routerInit(appctx.GetLogger(ctx)); err != nil {
		_ = s.Close()
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit(log *zerolog.Logger) error {
	limiter, err := s.Conf.RateLimit.NewLimiter(log)
	if err != nil {
		return err
	}
	s.limiter = limiter
	// Everything under this prefix is unauthenticated (see Unprotected).
	s.router.Use(limiter.Middleware)

	wkocmHandler := new(wkocmHandler)
	wkocmHandler.init(&s.Conf.OCMProvider)
	s.router.Get("/ocm", wkocmHandler.Ocm)
	return nil
}

func (s *svc) Close() error {
	s.limiter.Close()
	return nil
}

func (s *svc) Prefix() string {
	return "/.well-known"
}

func (s *svc) Unprotected() []string {
	return []string{"/", "/ocm"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Str("path", r.URL.Path).Msg(".well-known routing")

		// unset raw path, otherwise chi uses it to route and then fails to match percent encoded path segments
		r.URL.RawPath = ""
		s.router.ServeHTTP(w, r)
	})
}
