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

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the OCM API is served.
const mount = "/ocm"

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
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
	if c.Prefix == "" {
		c.Prefix = "ocm"
	}
	if c.TokenManager == "" {
		c.TokenManager = "jwt"
	}
}

type svc struct {
	Conf *config

	shares        *sharesHandler
	invites       *invitesHandler
	notifications *notifHandler
	token         *tokenHandler
}

// New returns a new ocmd object, that implements
// the OCM APIs specified in https://cs3org.github.io/OCM-API/docs.html
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{Conf: &c}

	if err := s.handlersInit(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) handlersInit() error {
	sharesHandler := new(sharesHandler)
	invitesHandler := new(invitesHandler)
	notifHandler := new(notifHandler)

	if err := sharesHandler.init(s.Conf); err != nil {
		return err
	}
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

	s.shares = sharesHandler
	s.invites = invitesHandler
	s.notifications = notifHandler
	s.token = tokenHandler
	return nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return mount
}

// Routes declares the OCM ingress endpoints. They authenticate at the protocol
// layer, so they stay reachable without the outer auth middleware.
func (s *svc) Routes(r *router.Router) {
	r.Group(mount, func(r *router.Router) {
		r.Post(sharesPath, s.shares.CreateShare, router.Unprotected())
		r.Post(inviteAcceptedPath, s.invites.AcceptInvite, router.Unprotected())
		r.Post(notificationsPath, s.notifications.Notifications, router.Unprotected())
		r.Post(tokenPath, s.token.ExchangeToken, router.Unprotected())
	})
}
