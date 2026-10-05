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

package ratelimit

import (
	"github.com/rs/zerolog"
)

// Defaults shared by all the services embedding ServiceConfig.
const (
	DefaultRequestsPerMinute = 60
	DefaultBurst             = 20
)

// ServiceConfig is the configuration block shared by the HTTP services that
// rate limit their unauthenticated routes. Embed it in a service config with
// `mapstructure:",squash"` so that its keys sit at the top level of the
// service section, and call ApplyDefaults from the service's ApplyDefaults.
type ServiceConfig struct {
	// UnauthRateLimit is the sustained number of requests per minute allowed
	// per client on the unauthenticated routes. 0 selects
	// DefaultRequestsPerMinute; a negative value disables limiting.
	UnauthRateLimit int `mapstructure:"unauth_rate_limit"`
	// UnauthRateLimitBurst is how many requests a client may send
	// back-to-back. 0 selects DefaultBurst.
	UnauthRateLimitBurst int `mapstructure:"unauth_rate_limit_burst"`
	// TrustedProxyCIDRs lists the reverse proxies whose X-Forwarded-For header
	// is trusted to identify the client. Without it, clients behind a proxy
	// all share the proxy's address and therefore a single bucket. Any
	// invalid entry aborts service initialization.
	TrustedProxyCIDRs []string `mapstructure:"trusted_proxy_cidrs"`
}

// ApplyDefaults fills in the default rate and burst.
func (c *ServiceConfig) ApplyDefaults() {
	if c.UnauthRateLimit == 0 {
		c.UnauthRateLimit = DefaultRequestsPerMinute
	}
	if c.UnauthRateLimitBurst == 0 {
		c.UnauthRateLimitBurst = DefaultBurst
	}
}

// NewLimiter builds the limiter described by c and logs the outcome. It
// returns a nil limiter, whose Middleware is a no-op, when limiting is
// disabled. Build it before any other service startup work so that an
// invalid trusted_proxy_cidrs entry fails fast.
func (c *ServiceConfig) NewLimiter(log *zerolog.Logger) (*Limiter, error) {
	if c.UnauthRateLimit < 0 {
		log.Warn().Msg("rate limiting of unauthenticated routes is disabled")
		return nil, nil
	}
	l, err := New(Config{
		RequestsPerMinute: c.UnauthRateLimit,
		Burst:             c.UnauthRateLimitBurst,
		TrustedProxyCIDRs: c.TrustedProxyCIDRs,
	})
	if err != nil {
		return nil, err
	}
	log.Info().
		Int("requests_per_minute", c.UnauthRateLimit).
		Int("burst", c.UnauthRateLimitBurst).
		Int("trusted_proxies", len(c.TrustedProxyCIDRs)).
		Msg("rate limiting unauthenticated routes")
	return l, nil
}
