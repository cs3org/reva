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
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

// mount is where the OCS API is served.
const mount = "/ocs"

func init() {
	global.Register("ocs", New)
}

type svc struct {
	c                  *config.Config
	warmupCacheTracker *ttlcache.Cache

	capabilities *capabilities.Handler
	user         *user.Handler
	users        *users.Handler
	config       *configHandler.Handler
	shares       *shares.Handler
	sharees      *sharees.Handler
}

func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config.Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{c: &c}

	log := appctx.GetLogger(ctx)
	if err := s.handlersInit(log); err != nil {
		return nil, err
	}

	if c.CacheWarmupDriver == "first-request" && c.ResourceInfoCacheTTL > 0 {
		s.warmupCacheTracker = ttlcache.NewCache()
		_ = s.warmupCacheTracker.SetTTL(time.Second * time.Duration(c.ResourceInfoCacheTTL))
	}

	return s, nil
}

func (s *svc) Prefix() string {
	return mount
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) handlersInit(l *zerolog.Logger) error {
	s.capabilities = new(capabilities.Handler)
	s.user = new(user.Handler)
	s.users = new(users.Handler)
	s.config = new(configHandler.Handler)
	s.shares = new(shares.Handler)
	s.sharees = new(sharees.Handler)

	s.capabilities.Init(s.c)
	s.users.Init(s.c)
	s.user.Init(s.c)
	s.config.Init(s.c)
	s.shares.Init(s.c, l)
	s.sharees.Init(s.c)
	return nil
}

// Routes declares the OCS API. The version is part of the path, so the tree is
// declared once per supported version rather than matched with a wildcard.
func (s *svc) Routes(r *router.Router) {
	for _, version := range []string{"1", "2"} {
		r.Group(mount+"/v"+version+".php", func(r *router.Router) {
			s.routes(r)
		}, response.VersionCtx(version), s.warmup)
	}
}

func (s *svc) routes(r *router.Router) {
	r.Group("/apps/files_sharing/api/v1", func(r *router.Router) {
		r.Group("/shares", func(r *router.Router) {
			r.Get("/", s.shares.ListShares)
			r.Options("/", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			r.Post("/", s.shares.CreateShare)

			r.Post("/pending/{shareid}", s.shares.AcceptReceivedShare)
			r.Delete("/pending/{shareid}", s.shares.RejectReceivedShare)

			r.Get("/remote_shares", s.shares.ListFederatedShares)
			r.Get("/remote_shares/{shareid}", s.shares.GetFederatedShare)

			r.Get("/{shareid}", s.shares.GetShare)
			r.Put("/{shareid}", s.shares.UpdateShare)
			r.Delete("/{shareid}", s.shares.RemoveShare)
			r.Post("/{shareid}/notify", s.shares.NotifyShare)
			// /shares/pending/{shareid} and /shares/{shareid}/notify both
			// match /shares/pending/notify without either being the more
			// specific one, so the overlap is declared explicitly. It resolves
			// the way it always has: as a share id of "notify" being accepted.
			r.Post("/pending/notify", s.shares.AcceptReceivedShare)
		})
		r.Get("/sharees", s.sharees.FindSharees)
	})

	r.Get("/config", s.config.GetConfig)

	r.Group("/cloud", func(r *router.Router) {
		r.Get("/capabilities", s.capabilities.GetCapabilities, router.Unprotected())
		r.Group("/user", func(r *router.Router) {
			r.Get("/", s.user.GetSelf)
			r.Patch("/", s.user.UpdateSelf)
			r.Get("/signing-key", s.user.SigningKey)
			r.Get("/clients", s.user.ListClients)
			r.Delete("/clients/{cid}", s.user.DeleteClient)
			r.Get("/login-flow/{lt}", s.user.LoginFlowInfo)
			r.Post("/login-flow/{lt}/grant", s.user.LoginFlowGrant)
			r.Post("/login-flow/{lt}/deny", s.user.LoginFlowDeny)
		})
		r.Group("/users", func(r *router.Router) {
			r.Get("/{userid}", s.users.GetUsers)
			r.Get("/{userid}/groups", s.users.GetGroups)
		})
	})
}

// warmup kicks off the share cache warmup for the user, which every OCS
// request did before it was routed.
func (s *svc) warmup(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		go s.cacheWarmup(w, r)
		next.ServeHTTP(w, r)
	})
}
