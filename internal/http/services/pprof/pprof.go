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

package pprof

import (
	"context"
	"net/http"
	"net/http/pprof"
	"path"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	global.Register("pprof", New)
}

// New returns a new pprof service.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

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

	if c.Prefix == "" {
		c.Prefix = "debug"
	}
}

type svc struct {
	conf *config
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{"/"}
}

func (s *svc) Handler() http.Handler {
	mux := http.NewServeMux()
	prefix := s.conf.Prefix
	mux.HandleFunc(path.Join(prefix, "/pprof/"), pprof.Index)
	mux.HandleFunc(path.Join(prefix, "/pprof/profile"), pprof.Profile)
	mux.HandleFunc(path.Join(prefix, "/pprof/symbol"), pprof.Symbol)
	mux.HandleFunc(path.Join(prefix, "/pprof/symbol"), pprof.Trace)
	//return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	//	m := nett
	//})
	return mux
}
