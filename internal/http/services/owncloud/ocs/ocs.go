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

package ocs

import (
	"context"
	"net/http"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/apps/sharing/sharees"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/cloud/capabilities"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/cloud/user"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/cloud/users"
	configHandler "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/handlers/config"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("ocs", New)
}

type svc struct {
	c                  *config.Config
	router             *chi.Mux
	warmupCacheTracker *ttlcache.Cache
}

func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config.Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	s := &svc{
		c:      &c,
		router: r,
	}

	log := appctx.GetLogger(ctx)
	if err := s.routerInit(log); err != nil {
		return nil, err
	}

	if c.CacheWarmupDriver == "first-request" && c.ResourceInfoCacheTTL > 0 {
		s.warmupCacheTracker = ttlcache.NewCache()
		_ = s.warmupCacheTracker.SetTTL(time.Second * time.Duration(c.ResourceInfoCacheTTL))
	}

	return s, nil
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{"/v1.php/cloud/capabilities", "/v2.php/cloud/capabilities"}
}

func (s *svc) routerInit(l *zerolog.Logger) error {
	capabilitiesHandler := new(capabilities.Handler)
	userHandler := new(user.Handler)
	usersHandler := new(users.Handler)
	configHandler := new(configHandler.Handler)
	sharesHandler := new(shares.Handler)
	shareesHandler := new(sharees.Handler)
	capabilitiesHandler.Init(s.c)
	usersHandler.Init(s.c)
	userHandler.Init(s.c)
	configHandler.Init(s.c)
	sharesHandler.Init(s.c, l)
	shareesHandler.Init(s.c)

	s.router.Route("/v{version:(1|2)}.php", func(r chi.Router) {
		r.Use(response.VersionCtx)
		r.Route("/apps/files_sharing/api/v1", func(r chi.Router) {
			r.Route("/shares", func(r chi.Router) {
				r.Get("/", sharesHandler.ListShares)
				r.Options("/", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
				r.Post("/", sharesHandler.CreateShare)
				r.Route("/pending/{shareid}", func(r chi.Router) {
					r.Post("/", sharesHandler.AcceptReceivedShare)
					r.Delete("/", sharesHandler.RejectReceivedShare)
				})
				r.Route("/remote_shares", func(r chi.Router) {
					r.Get("/", sharesHandler.ListFederatedShares)
					r.Get("/{shareid}", sharesHandler.GetFederatedShare)
				})
				r.Get("/{shareid}", sharesHandler.GetShare)
				r.Put("/{shareid}", sharesHandler.UpdateShare)
				r.Get("/{shareid}/notify", sharesHandler.NotifyShare)
				r.Delete("/{shareid}", sharesHandler.RemoveShare)
			})
			r.Get("/sharees", shareesHandler.FindSharees)
		})

		r.Get("/config", configHandler.GetConfig)

		r.Route("/cloud", func(r chi.Router) {
			r.Get("/capabilities", capabilitiesHandler.GetCapabilities)
			r.Route("/user", func(r chi.Router) {
				r.Get("/", userHandler.GetSelf)
				r.Patch("/", userHandler.UpdateSelf)
				r.Get("/signing-key", userHandler.SigningKey)
			})
			r.Route("/users", func(r chi.Router) {
				r.Get("/{userid}", usersHandler.GetUsers)
				r.Get("/{userid}/groups", usersHandler.GetGroups)
			})
		})
	})
	return nil
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Str("path", r.URL.Path).Msg("ocs routing")

		// Warmup the share cache for the user
		go s.cacheWarmup(w, r)

		// unset raw path, otherwise chi uses it to route and then fails to match percent encoded path segments
		r.URL.RawPath = ""
		s.router.ServeHTTP(w, r)
	})
}
