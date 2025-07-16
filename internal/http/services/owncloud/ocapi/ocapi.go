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
	"github.com/cs3org/reva/v3/pkg/trace"
	"github.com/go-chi/chi/v5"
)

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
	r := chi.NewRouter()

	r.Post("/v0/settings/roles-list", staticResponse(roles))
	r.Post("/v0/settings/assignments-list", staticResponse(assignments))
	r.Post("/v0/settings/permissions-list", staticResponse(permissions))
	r.Post("/v0/settings/values-list", staticResponse(values))

	return svc{r: r}, nil
}

func staticResponse(content string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", trace.Get(r.Context()))
		_, _ = w.Write([]byte(content))
	})
}

type svc struct {
	r *chi.Mux
}

func (s svc) Handler() http.Handler {
	return s.r
}

func (s svc) Prefix() string { return "api" }

func (s svc) Close() error { return nil }

func (s svc) Unprotected() []string { return []string{"/"} }
