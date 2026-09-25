// Copyright 2018-2025 CERN
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

package pprof

import (
	"context"
	"net/http"
	"net/http/pprof"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
)

// mount is where the profiling endpoints are served.
const mount = "/debug"

func init() {
	global.Register("pprof", New)
}

// New returns a new pprof service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	return &svc{}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type svc struct{}

// Routes mounts the standard library's pprof endpoints rather than declaring
// them one by one: pprof.Index derives the profile name from the request path,
// and only recognizes it under /debug/pprof/, so it needs the path untouched.
func (s *svc) Routes(r *router.Router) {
	mux := http.NewServeMux()
	mux.HandleFunc(mount+"/pprof/", pprof.Index)
	mux.HandleFunc(mount+"/pprof/profile", pprof.Profile)
	mux.HandleFunc(mount+"/pprof/symbol", pprof.Symbol)
	mux.HandleFunc(mount+"/pprof/trace", pprof.Trace)
	// See https://pkg.go.dev/runtime/pprof#Profile for predefined profile names.
	mux.HandleFunc(mount+"/pprof/heap", func(w http.ResponseWriter, r *http.Request) { pprof.Handler("heap").ServeHTTP(w, r) })
	mux.HandleFunc(mount+"/pprof/goroutine", func(w http.ResponseWriter, r *http.Request) { pprof.Handler("goroutine").ServeHTTP(w, r) })

	r.Mount(mount, mux, router.Unprotected())
}
