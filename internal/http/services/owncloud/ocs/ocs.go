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

package ocs

import (
	"context"
	"net/http"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/sharees"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/apps/sharing/shares"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/capabilities"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/user"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/cloud/users"
	configHandler "github.com/cs3org/reva/internal/http/services/owncloud/ocs/handlers/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/rs/zerolog"
)

const name = "ocs"

func init() {
	rhttp.Register(name, New)
}

type svc struct {
	c                   *config.Config
	warmupCacheTracker  *ttlcache.Cache
	log                 *zerolog.Logger
	capabilitiesHandler *capabilities.Handler
	userHandler         *user.Handler
	usersHandler        *users.Handler
	configHandler       *configHandler.Handler
	sharesHandler       *shares.Handler
	shareesHandler      *sharees.Handler
}

func New(ctx context.Context, m map[string]interface{}) (rhttp.Service, error) {
	var c config.Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	s := &svc{
		c:   &c,
		log: log,
	}

	if c.CacheWarmupDriver == "first-request" && c.ResourceInfoCacheTTL > 0 {
		s.warmupCacheTracker = ttlcache.NewCache()
		_ = s.warmupCacheTracker.SetTTL(time.Second * time.Duration(c.ResourceInfoCacheTTL))
	}

	return s, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Register(r mux.Router) {
	s.capabilitiesHandler = new(capabilities.Handler)
	s.userHandler = new(user.Handler)
	s.usersHandler = new(users.Handler)
	s.configHandler = new(configHandler.Handler)
	s.sharesHandler = new(shares.Handler)
	s.shareesHandler = new(sharees.Handler)
	s.capabilitiesHandler.Init(s.c)
	s.usersHandler.Init(s.c)
	s.userHandler.Init(s.c)
	s.configHandler.Init(s.c)
	s.sharesHandler.Init(s.c, s.log)
	s.shareesHandler.Init(s.c)

	r.Route("/ocs/:version", func(r mux.Router) {
		r.Route("/apps/files_sharing/api/v1", func(r mux.Router) {
			r.Route("/shares", func(r mux.Router) {
				r.Get("", http.HandlerFunc(s.sharesHandler.ListShares))
				r.Options("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				r.Post("", http.HandlerFunc(s.sharesHandler.CreateShare))
				r.Route("/pending/:shareid", func(r mux.Router) {
					r.Post("", http.HandlerFunc((s.sharesHandler.AcceptReceivedShare)))
					r.Delete("", http.HandlerFunc(s.sharesHandler.RejectReceivedShare))
				})
				r.Route("/remote_shares", func(r mux.Router) {
					r.Get("", http.HandlerFunc(s.sharesHandler.ListFederatedShares))
					r.Get("/:shareid", http.HandlerFunc(s.sharesHandler.GetFederatedShare))
				})
				r.Get("/:shareid", http.HandlerFunc(s.sharesHandler.GetShare))
				r.Put("/:shareid", http.HandlerFunc(s.sharesHandler.UpdateShare))
				r.Get("/:shareid/notify", http.HandlerFunc(s.sharesHandler.NotifyShare))
				r.Delete("/:shareid", http.HandlerFunc(s.sharesHandler.RemoveShare))
			})
			r.Get("/sharees", http.HandlerFunc(s.shareesHandler.FindSharees))
		})

		r.Get("/config", http.HandlerFunc(s.configHandler.GetConfig))

		r.Route("/cloud", func(r mux.Router) {
			r.Get("/capabilities", http.HandlerFunc(s.capabilitiesHandler.GetCapabilities), mux.Unprotected())
			r.Get("/user", http.HandlerFunc(s.userHandler.GetSelf))
			r.Patch("/user", http.HandlerFunc(s.userHandler.UpdateSelf))
			r.Route("/users", func(r mux.Router) {
				r.Get("/:userid", http.HandlerFunc(s.usersHandler.GetUsers))
				r.Get("/:userid/groups", http.HandlerFunc(s.usersHandler.GetGroups))
			})
		})
	}, mux.WithMiddleware(response.VersionCtx), mux.WithMiddleware(s.cacheWarmupMiddleware))
}

func (s *svc) Close() error {
	return nil
}
