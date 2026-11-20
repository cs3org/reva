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

package ocapi

import (
	"context"
	_ "embed"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// mount is where the settings API is served.
const mount = "/api"

// This API exposes all supported roles/assignments/permissions/values by the system,
// and as such it provides static content.

//go:embed roles.json
var roles string

//go:embed assignments.json
var assignments string

//go:embed permissions.json
var permissions string

//go:embed values.json
var values string

func init() {
	global.Register("ocapi", New)
}

func New(ctx context.Context, m map[string]any) (global.Service, error) {
	return svc{}, nil
}

func staticResponse(content string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(content))
	})
}

type svc struct{}

func (s svc) Routes(r *router.Router) {
	r.Group(mount+"/v0/settings", func(r *router.Router) {
		r.Post("/roles-list", staticResponse(roles), router.Unprotected())
		r.Post("/assignments-list", staticResponse(assignments), router.Unprotected())
		r.Post("/permissions-list", staticResponse(permissions), router.Unprotected())
		r.Post("/values-list", staticResponse(values), router.Unprotected())
	})
}

func (s svc) Close() error { return nil }
