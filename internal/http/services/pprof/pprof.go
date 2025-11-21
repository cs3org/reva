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
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	global.Register("pprof", New)
}

// New returns a new pprof service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	c.ApplyDefaults()

	return &svc{conf: &c}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

func (c *config) ApplyDefaults() {
	// pprof is always exposed at /debug
	c.Prefix = "debug"
}

type svc struct {
	conf *config
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{"/", "/pprof/", "/pprof/profile", "/pprof/symbol", "/pprof/trace", "/pprof/heap", "/pprof/goroutine"}
}

func (s *svc) Handler() http.Handler {
	mux := http.NewServeMux()
	// example: /debug/pprof/profile
	mux.HandleFunc("/pprof/", pprof.Index)
	mux.HandleFunc("/pprof/profile", pprof.Profile)
	mux.HandleFunc("/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/pprof/trace", pprof.Trace)
	// See https://pkg.go.dev/runtime/pprof#Profile for predefined profile names.
	mux.HandleFunc("/pprof/heap", func(w http.ResponseWriter, r *http.Request) { pprof.Handler("heap").ServeHTTP(w, r) })
	mux.HandleFunc("/pprof/goroutine", func(w http.ResponseWriter, r *http.Request) { pprof.Handler("goroutine").ServeHTTP(w, r) })

	return mux
}
