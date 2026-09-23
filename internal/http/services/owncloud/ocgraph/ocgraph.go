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
	"net/url"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the Graph API is served.
const mount = "/graph"

func init() {
	global.Register("ocgraph", New)
}

type config struct {
	GatewaySvc                 string `mapstructure:"gatewaysvc"  validate:"required"`
	OCMEnabled                 bool   `mapstructure:"ocm_enabled"`
	WebDavBase                 string `mapstructure:"webdav_base"`
	WebBase                    string `mapstructure:"web_base"`
	BaseURL                    string `mapstructure:"base_url"    validate:"required"`
	PubRWLinkMaxExpiration     int64  `mapstructure:"pub_rw_link_max_expiration"`
	PubRWLinkDefaultExpiration int64  `mapstructure:"pub_rw_link_default_expiration"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.WebBase == "" {
		c.WebBase, _ = url.JoinPath(c.BaseURL, "/files/spaces")
	}

	if c.WebDavBase == "" {
		c.WebDavBase, _ = url.JoinPath(c.BaseURL, "/remote.php/dav/spaces")
	}
}

// ListResponse is used for proper marshalling of Graph list responses
type ListResponse struct {
	Value any `json:"value,omitempty"`
}

type svc struct {
	c *config
}

func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		c: &c,
	}
	return s, nil
}

func (s *svc) Routes(r *router.Router) {
	r.Group(mount+"/v1.0", func(r *router.Router) {
		r.Get("/me", s.getMe)
		r.Patch("/me", s.patchMe)
		r.Get("/drives/{spaceID}", s.getSpace)
		r.Patch("/drives/{spaceID}", s.patchSpace)
		r.Get("/users", s.listUsers)
		r.Get("/groups", s.listGroups)
	})

	r.Group(mount+"/v1beta1", func(r *router.Router) {
		r.Get("/me/drives", s.listMySpaces)
		r.Get("/me/drive/sharedWithMe", s.getSharedWithMe)
		r.Get("/me/drive/sharedByMe", s.getSharedByMe)
		r.Get("/roleManagement/permissions/roleDefinitions", s.getRoleDefinitions)
		r.Group("/drives/{spaceID}", func(r *router.Router) {
			r.Get("/root/permissions", s.getRootDrivePermissions)
			r.Group("/items/{resourceID}", func(r *router.Router) {
				r.Patch("/", s.updateReceivedShare)
				r.Post("/invite", s.share)
				r.Post("/createLink", s.createLink)
				r.Get("/permissions", s.getDrivePermissions)
				r.Patch("/permissions/{shareID}", s.updateDrivePermissions)
				r.Delete("/permissions/{shareID}", s.deleteDrivePermissions)
				r.Post("/permissions/{shareID}/setPassword", s.updateLinkPassword)
			})
		})
	})
}

func (s *svc) Prefix() string { return mount }

func (s *svc) Close() error { return nil }
