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

package reverseproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "reverseproxy"

func init() {
	global.Register(name, New)
}

type proxyRule struct {
	Endpoint string `mapstructure:"endpoint" json:"endpoint"`
	Backend  string `mapstructure:"backend" json:"backend"`
}

type config struct {
	ProxyRulesJSON string `mapstructure:"proxy_rules_json"`
}

func (c *config) ApplyDefaults() {
	if c.ProxyRulesJSON == "" {
		c.ProxyRulesJSON = "/etc/revad/proxy_rules.json"
	}
}

type svc struct {
	rules []proxyRule
}

// New returns an instance of the reverse proxy service.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	f, err := os.Open(c.ProxyRulesJSON)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var rules []proxyRule
	if err := json.NewDecoder(f).Decode(&rules); err != nil {
		return nil, err
	}

	return &svc{rules: rules}, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Register(r mux.Router) {
	for _, rule := range s.rules {
		remote, err := url.Parse(rule.Backend)
		if err != nil {
			// Skip the rule if the backend is not a valid URL
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(remote)
		r.Handle(rule.Endpoint, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Host = remote.Host
			if token, ok := ctxpkg.ContextGetToken(r.Context()); ok {
				r.Header.Set(ctxpkg.TokenHeader, token)
			}
			proxy.ServeHTTP(w, r)
		}))
	}
}
