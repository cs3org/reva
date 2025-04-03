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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"context"
	"net/http"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
)

func init() {
	global.Register("ocgraph", New)
}

type config struct {
	GatewaySvc string `mapstructure:"gatewaysvc"  validate:"required"`
	WebDavBase string `mapstructure:"webdav_base"`
	WebBase    string `mapstructure:"web_base"`
	BaseURL    string `mapstructure:"base_url"    validate:"required"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.WebBase == "" {
		c.WebBase = path.Join(c.BaseURL, "/files/spaces")
	}

	if c.WebDavBase == "" {
		c.WebDavBase = path.Join(c.BaseURL, "/remote.php/dav/spaces")
	}
}

// ListResponse is used for proper marshalling of Graph list responses
type ListResponse struct {
	Value interface{} `json:"value,omitempty"`
}

type svc struct {
	c      *config
	router *chi.Mux
}

func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		c: &c,
	}
	s.initRouter()

	return s, nil
}

func (s *svc) initRouter() {
	s.router = chi.NewRouter()

	s.router.Route("/v1.0", func(r chi.Router) {
		r.Route("/me", func(r chi.Router) {
			r.Get("/", s.getMe)
		})
		r.Route("/drives", func(r chi.Router) {
			r.Get("/{space-id}", s.getSpace)
		})
		r.Route("/users", func(r chi.Router) {
			r.Get("/", s.listUsers)
		})
		r.Route("/groups", func(r chi.Router) {
			r.Get("/", s.listGroups)
		})
	})
	s.router.Route("/v1beta1", func(r chi.Router) {
		r.Route("/me", func(r chi.Router) {
			r.Route("/drives", func(r chi.Router) {
				r.Get("/", s.listMySpaces)

			})
		})
		r.Route("/me/drive", func(r chi.Router) {
			r.Get("/sharedWithMe", s.getSharedWithMe)
			r.Get("/sharedByMe", s.getSharedByMe)
		})
		r.Get("/roleManagement/permissions/roleDefinitions", s.getRoleDefinitions)
		r.Get("/drives/{space-id}/root/permissions", s.getRootDrivePermissions)
		r.Get("/drives/{space-id}/items/{resource-id}/permissions", s.getDrivePermissions)
		r.Post("/drives/{space-id}/items/{resource-id}/invite", s.share)
		r.Post("/drives/{space-id}/items/{resource-id}/createLink", s.createLink)
	})
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
}

func (s *svc) Handler() http.Handler { return s.router }

func (s *svc) Prefix() string { return "graph" }

func (s *svc) Close() error { return nil }

func (s *svc) Unprotected() []string { return nil }

func handleError(err error, w http.ResponseWriter) {
	w.WriteHeader(http.StatusInternalServerError)
<<<<<<< HEAD
	w.Write([]byte(err.Error()))
=======
>>>>>>> 2f58acbb1 (Implemented v0.1 of listUsers libregraph endpoint)
}

func handleRpcStatus(ctx context.Context, status *rpcv1beta1.Status, w http.ResponseWriter) {
	log := appctx.GetLogger(ctx)
	log.Error().Str("Status", status.String()).Msg("Failed to contact gateway in listUsers")

	switch status.Code {
	case rpcv1beta1.Code_CODE_PERMISSION_DENIED:
		w.WriteHeader(http.StatusForbidden)
	case rpcv1beta1.Code_CODE_UNAUTHENTICATED:
		w.WriteHeader(http.StatusUnauthorized)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
}
