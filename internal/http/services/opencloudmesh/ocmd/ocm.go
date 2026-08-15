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

package ocmd

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
)

func init() {
	global.Register("ocm", New)
}

type config struct {
	Prefix                     string                    `mapstructure:"prefix"`
	GatewaySvc                 string                    `mapstructure:"gatewaysvc"                    validate:"required"`
	ExposeRecipientDisplayName bool                      `mapstructure:"expose_recipient_display_name"`
	TokenManager               string                    `mapstructure:"token_manager"`
	TokenManagers              map[string]map[string]any `mapstructure:"token_managers"`
	// MachineSecret is the shared secret used to impersonate the recipient of an
	// incoming OCM share when auto-registering its remote sender/owner as accepted users.
	MachineSecret string `mapstructure:"machine_secret"`
	// AutoAcceptProviders is a list of regular expressions matched against the sender's
	// provider domain. A match activates auto-registration of the share's remote users
	// for all OCM share types (embedded shares are always auto-registered).
	AutoAcceptProviders []string `mapstructure:"auto_accept_providers"`
	// TrustForwardedFor reads the sender IP from X-Forwarded-For. Enable it only
	// behind a reverse proxy that sets the header, else peers can spoof it.
	TrustForwardedFor bool `mapstructure:"trust_forwarded_for"`
	// OCMClientInsecure skips TLS verification when probing a remote provider's
	// discovery endpoint. Off by default; turning it on exposes discovery to MITM.
	OCMClientInsecure bool `mapstructure:"ocm_client_insecure"`
	// OCMClientResponseLimit caps outbound OCM JSON response bodies in bytes.
	// Zero means 1 MiB.
	OCMClientResponseLimit int64 `mapstructure:"ocm_client_response_limit"`
	// OCMClientTLSMinVersion is the untrusted-client TLS minimum enum string.
	// Empty means TLS 1.2. Accepted values are "1.2" and "1.3".
	OCMClientTLSMinVersion string `mapstructure:"ocm_client_tls_min_version"`
	// UntrustedClientSecurity is the shared hatch and redirect policy for
	// untrusted outbound clients used by this service (TOML block
	// ocm_client_security). Non-received consumers must keep the hatch closed
	// (no allow_http / allowed_cidrs).
	UntrustedClientSecurity UntrustedClientSecurity `mapstructure:"ocm_client_security"`
	ocmClientTLSMin         uint16
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	if c.Prefix == "" {
		c.Prefix = "ocm"
	}
	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}
	if c.OCMClientResponseLimit == 0 {
		c.OCMClientResponseLimit = 1 << 20
	}
}

type svc struct {
	Conf   *config
	router chi.Router
	shares *sharesHandler
}

// New returns a new ocmd object, that implements
// the OCM APIs specified in https://cs3org.github.io/OCM-API/docs.html
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	c.UntrustedClientSecurity.ApplyDefaults()
	if err := c.UntrustedClientSecurity.Compile(); err != nil {
		return nil, err
	}
	if err := c.UntrustedClientSecurity.RejectHatch(); err != nil {
		return nil, err
	}
	minVersion, err := ParseTLSMinVersion(c.OCMClientTLSMinVersion)
	if err != nil {
		return nil, err
	}
	c.ocmClientTLSMin = minVersion

	r := chi.NewRouter()
	s := &svc{
		Conf:   &c,
		router: r,
	}

	if err := s.routerInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) routerInit() error {
	sharesHandler := new(sharesHandler)
	invitesHandler := new(invitesHandler)
	notifHandler := new(notifHandler)

	if err := sharesHandler.init(s.Conf); err != nil {
		return err
	}
	s.shares = sharesHandler
	if err := invitesHandler.init(s.Conf); err != nil {
		return err
	}
	if err := notifHandler.init(s.Conf); err != nil {
		return err
	}

	tokenHandler := new(tokenHandler)
	if err := tokenHandler.init(s.Conf); err != nil {
		return err
	}

	s.router.Post(sharesPath, sharesHandler.CreateShare)
	s.router.Post(inviteAcceptedPath, invitesHandler.AcceptInvite)
	s.router.Post(notificationsPath, notifHandler.Notifications)
	s.router.Post(tokenPath, tokenHandler.ExchangeToken)
	return nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.Conf.Prefix
}

func (s *svc) Unprotected() []string {
	// These OCM ingress routes authenticate at the protocol layer, so they stay
	// reachable without the outer auth middleware.
	return []string{inviteAcceptedPath, sharesPath, notificationsPath, tokenPath}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Str("path", r.URL.Path).Msg("ocm routing")

		// unset raw path, otherwise chi uses it to route and then fails to match percent encoded path segments
		r.URL.RawPath = ""
		s.router.ServeHTTP(w, r)
	})
}
