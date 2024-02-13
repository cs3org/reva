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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"context"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
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
	WebDavBase string `mapstructure:"webdav_base" validate:"required"`
	WebBase    string `mapstructure:"web_base"    validate:"required"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
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

	r := chi.NewRouter()
	s := &svc{
		c:      &c,
		router: r,
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit() error {
	s.router.Route("/v1.0", func(r chi.Router) {
		r.Route("/me", func(r chi.Router) {
			r.Get("/", s.getMe)
			r.Route("/drives", func(r chi.Router) {
				r.Get("/", s.listMySpaces)

			})
		})
		r.Route("/drives", func(r chi.Router) {
			r.Get("/{space-id}", s.getSpace)
		})
	})
	return nil
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
}

func (s *svc) Handler() http.Handler { return s.router }

func (s *svc) Prefix() string { return "graph" }

func (s *svc) Close() error { return nil }

func (s *svc) Unprotected() []string { return nil }
