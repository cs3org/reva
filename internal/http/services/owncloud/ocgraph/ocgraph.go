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
	libregraph "github.com/owncloud/libre-graph-api-go"
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
		r.Get("/me", s.getMe, router.Describe(router.Doc{
			Summary:   "The signed-in user",
			Responses: map[int]any{200: libregraph.User{}},
		}))
		r.Patch("/me", s.patchMe, router.Describe(router.Doc{
			Summary:   "Update the signed-in user",
			Body:      libregraph.User{},
			Responses: map[int]any{200: libregraph.User{}},
		}))
		r.Get("/drives/{spaceID}", s.getSpace, router.Describe(router.Doc{
			Summary:   "A space",
			Responses: map[int]any{200: libregraph.Drive{}},
		}))
		r.Patch("/drives/{spaceID}", s.patchSpace, router.Describe(router.Doc{
			Summary:   "Update a space",
			Body:      libregraph.DriveUpdate{},
			Responses: map[int]any{200: libregraph.Drive{}},
		}))
		r.Get("/users", s.listUsers, router.Describe(router.Doc{
			Summary: "Search users",
			Params: []router.Param{
				router.Query("$search", `A search term, quoted as "term"`).WithExample(`"einstein"`),
			},
			Responses: map[int]any{200: ListResponse{}},
		}))
		r.Get("/groups", s.listGroups, router.Describe(router.Doc{
			Summary: "Search groups",
			Params: []router.Param{
				router.Query("$search", `A search term, quoted as "term"`),
			},
			Responses: map[int]any{200: ListResponse{}},
		}))
	})

	r.Group(mount+"/v1beta1", func(r *router.Router) {
		r.Get("/me/drives", s.listMySpaces, router.Describe(router.Doc{
			Summary:   "The spaces the signed-in user can reach",
			Responses: map[int]any{200: ListResponse{}},
		}))
		r.Get("/me/drive/sharedWithMe", s.getSharedWithMe, router.Describe(router.Doc{
			Summary:   "Shares received by the signed-in user",
			Responses: map[int]any{200: ListResponse{}},
		}))
		r.Get("/me/drive/sharedByMe", s.getSharedByMe, router.Describe(router.Doc{
			Summary:   "Shares created by the signed-in user",
			Responses: map[int]any{200: ListResponse{}},
		}))
		r.Get("/roleManagement/permissions/roleDefinitions", s.getRoleDefinitions, router.Describe(router.Doc{
			Summary:   "The roles a share can be given",
			Responses: map[int]any{200: []libregraph.UnifiedRoleDefinition{}},
		}))
		r.Group("/drives/{spaceID}", func(r *router.Router) {
			r.Get("/root/permissions", s.getRootDrivePermissions, router.Describe(router.Doc{
				Summary:   "The permissions on a space's root",
				Responses: map[int]any{200: ListResponse{}},
			}))
			r.Group("/items/{resourceID}", func(r *router.Router) {
				r.Patch("/", s.updateReceivedShare, router.Describe(router.Doc{
					Summary:   "Accept or decline a received share",
					Body:      libregraph.DriveItem{},
					Responses: map[int]any{200: libregraph.DriveItem{}},
				}))
				r.Post("/invite", s.share, router.Describe(router.Doc{
					Summary:   "Share a resource with users or groups",
					Body:      libregraph.DriveItemInvite{},
					Responses: map[int]any{200: ListResponse{}},
				}))
				r.Post("/createLink", s.createLink, router.Describe(router.Doc{
					Summary:   "Create a public link to a resource",
					Body:      libregraph.DriveItemCreateLink{},
					Responses: map[int]any{200: libregraph.Permission{}},
				}))
				r.Get("/permissions", s.getDrivePermissions, router.Describe(router.Doc{
					Summary:   "The permissions on a resource",
					Responses: map[int]any{200: ListResponse{}},
				}))
				r.Patch("/permissions/{shareID}", s.updateDrivePermissions, router.Describe(router.Doc{
					Summary:   "Update a share or a link",
					Body:      libregraph.Permission{},
					Responses: map[int]any{200: libregraph.Permission{}},
				}))
				r.Delete("/permissions/{shareID}", s.deleteDrivePermissions, router.Describe(router.Doc{
					Summary:   "Remove a share or a link",
					Responses: map[int]any{204: nil},
				}))
				r.Post("/permissions/{shareID}/setPassword", s.updateLinkPassword, router.Describe(router.Doc{
					Summary:   "Set or clear a public link's password",
					Body:      libregraph.SharingLinkPassword{},
					Responses: map[int]any{200: libregraph.Permission{}},
				}))
			})
		})
	})
}

func (s *svc) Close() error { return nil }
