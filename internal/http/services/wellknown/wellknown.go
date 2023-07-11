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

package wellknown

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "wellknown"

func init() {
	global.Register("wellknown", New)
}

type config struct {
	Issuer                string `mapstructure:"issuer"`
	AuthorizationEndpoint string `mapstructure:"authorization_endpoint"`
	JwksURI               string `mapstructure:"jwks_uri"`
	TokenEndpoint         string `mapstructure:"token_endpoint"`
	RevocationEndpoint    string `mapstructure:"revocation_endpoint"`
	IntrospectionEndpoint string `mapstructure:"introspection_endpoint"`
	UserinfoEndpoint      string `mapstructure:"userinfo_endpoint"`
	EndSessionEndpoint    string `mapstructure:"end_session_endpoint"`
}

type svc struct {
	conf *config
}

// New returns a new webuisvc.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		conf: &c,
	}
	return s, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Register(r mux.Router) {
	r.Route("/.well-known", func(r mux.Router) {
		r.Get("/webfinger", http.HandlerFunc(s.doWebfinger))
		r.Get("/openid-configuration", http.HandlerFunc(s.doOpenidConfiguration), mux.Unprotected())
	})
}

func (s *svc) Close() error {
	return nil
}
