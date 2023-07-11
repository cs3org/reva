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

package ocmd

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "ocmd"

func init() {
	global.Register(name, New)
}

type config struct {
	GatewaySvc                 string `mapstructure:"gatewaysvc"                    validate:"required"`
	ExposeRecipientDisplayName bool   `mapstructure:"expose_recipient_display_name"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf           *config
	sharesHandler  *sharesHandler
	invitesHandler *invitesHandler
	notifHandler   *notifHandler
}

// New returns a new ocmd object, that implements
// the OCM APIs specified in https://cs3org.github.io/OCM-API/docs.html
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{conf: &c}

	if err := s.initServices(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) initServices() error {
	s.sharesHandler = new(sharesHandler)
	s.invitesHandler = new(invitesHandler)
	s.notifHandler = new(notifHandler)

	if err := s.sharesHandler.init(s.conf); err != nil {
		return err
	}
	if err := s.invitesHandler.init(s.conf); err != nil {
		return err
	}
	if err := s.notifHandler.init(s.conf); err != nil {
		return err
	}
	return nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Register(r mux.Router) {
	r.Route("/ocm", func(r mux.Router) {
		r.Post("/shares", http.HandlerFunc(s.sharesHandler.CreateShare))
		r.Post("/invite-accepted", http.HandlerFunc(s.invitesHandler.AcceptInvite))
		r.Post("/notifications", http.HandlerFunc(s.notifHandler.Notifications))
	}, mux.Unprotected())
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}
