// Copyright 2018-2026 CERN
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

// Package apppassword provides a credential strategy for an app password sent
// alone in a bearer header. It emits credentials of type "appauth" without a
// client id, so the appauth provider finds the owner from the app password.
package apppassword

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/registry"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/ratelimit"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func init() {
	registry.Register("apppassword", New)
}

type config struct {
	FailuresPerIPPerMin int  `mapstructure:"failures_per_ip_per_min" docs:"60;Wrong app passwords a client IP may send per minute before it gets 429. A negative value disables the limit."`
	TrustForwardedFor   bool `mapstructure:"trust_forwarded_for" docs:"false;Take the client IP from X-Forwarded-For, behind a reverse proxy."`
}

func (c *config) ApplyDefaults() {
	if c.FailuresPerIPPerMin == 0 {
		c.FailuresPerIPPerMin = 60
	}
}

type strategy struct {
	c        config
	failures *ratelimit.Limiter
}

// New returns the apppassword credential strategy.
func New(m map[string]any) (auth.CredentialStrategy, error) {
	s := &strategy{failures: ratelimit.New()}
	if err := cfg.Decode(m, &s.c); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *strategy) GetCredentials(_ http.ResponseWriter, r *http.Request) (*auth.Credentials, error) {
	secret, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	// Reva and OIDC tokens are JWTs, which contain dots; an app password never
	// does.
	if !ok || secret == "" || strings.Contains(secret, ".") {
		return nil, errors.New("no app password in a bearer header")
	}
	return &auth.Credentials{Type: "appauth", ClientSecret: secret, Presentation: string(scope.PresentationBearer)}, nil
}

func (s *strategy) AddWWWAuthenticate(w http.ResponseWriter, r *http.Request, realm string) {
	if realm == "" {
		realm = r.Host
	}
	w.Header().Add("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, realm))
}

// Throttled reports whether the client sent too many wrong app passwords.
func (s *strategy) Throttled(r *http.Request) bool {
	return s.failures.Exhausted(s.clientIP(r), s.c.FailuresPerIPPerMin)
}

// Failed records a wrong app password sent by the client.
func (s *strategy) Failed(r *http.Request) {
	s.failures.Allow(s.clientIP(r), s.c.FailuresPerIPPerMin)
}

func (s *strategy) clientIP(r *http.Request) string {
	ip, err := utils.GetClientIP(r, s.c.TrustForwardedFor)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
