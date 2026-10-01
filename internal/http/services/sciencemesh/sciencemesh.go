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

package sciencemesh

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/smtpclient"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the ScienceMesh API is served.
const mount = "/sciencemesh"

func init() {
	global.Register("sciencemesh", New)
}

// New returns a new sciencemesh service, which serves as the backend for the ScienceMesh web app
// to handle OCM related requests for local users.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{conf: &c}

	if err := s.handlersInit(); err != nil {
		return nil, err
	}

	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	SMTPCredentials      *smtpclient.SMTPCredentials `mapstructure:"smtp_credentials"`
	GatewaySvc           string                      `mapstructure:"gatewaysvc"         validate:"required"`
	MeshDirectoryURL     string                      `mapstructure:"mesh_directory_url" validate:"required"`
	ProviderDomain       string                      `mapstructure:"provider_domain"    validate:"required"`
	SubjectTemplate      string                      `mapstructure:"subject_template"`
	BodyTemplatePath     string                      `mapstructure:"body_template_path"`
	OCMMountPoint        string                      `mapstructure:"ocm_mount_point"`
	DirectoryServiceURLs string                      `mapstructure:"directory_service_urls"`
	OCMClientTimeout     int                         `mapstructure:"ocm_client_timeout"`
	OCMClientInsecure    bool                        `mapstructure:"ocm_client_insecure"`
}

func (c *config) ApplyDefaults() {
	if c.OCMMountPoint == "" {
		c.OCMMountPoint = "/ocm"
	}
	if c.OCMClientTimeout == 0 {
		c.OCMClientTimeout = 10
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf *config

	token     *tokenHandler
	providers *providersHandler
	apps      *appsHandler
	wayf      *wayfHandler
	embedded  *embeddedHandler
}

func (s *svc) handlersInit() error {
	tokenHandler := new(tokenHandler)
	if err := tokenHandler.init(s.conf); err != nil {
		return err
	}
	providersHandler := new(providersHandler)
	if err := providersHandler.init(s.conf); err != nil {
		return err
	}

	appsHandler := new(appsHandler)
	if err := appsHandler.init(s.conf); err != nil {
		return err
	}

	wayfHandler := new(wayfHandler)
	if err := wayfHandler.init(s.conf); err != nil {
		return err
	}
	embeddedHandler := new(embeddedHandler)
	if err := embeddedHandler.init(s.conf); err != nil {
		return err
	}

	s.token = tokenHandler
	s.providers = providersHandler
	s.apps = appsHandler
	s.wayf = wayfHandler
	s.embedded = embeddedHandler
	return nil
}

// Routes declares the ScienceMesh endpoints. Discovery is reachable without
// credentials, everything else is behind the auth middleware.
func (s *svc) Routes(r *router.Router) {
	r.Group(mount, func(r *router.Router) {
		r.Post("/generate-invite", s.token.Generate)
		r.Get("/list-invite", s.token.ListInvite)
		r.Post("/accept-invite", s.token.AcceptInvite)
		r.Get("/find-accepted-users", s.token.FindAccepted)
		r.Delete("/delete-accepted-user", s.token.DeleteAccepted)
		r.Get("/list-providers", s.providers.ListProviders)
		r.Post("/open-in-app", s.apps.OpenInApp)
		r.Get("/federations", s.wayf.GetFederations, router.Unprotected())
		r.Post("/discover", s.wayf.DiscoverProvider, router.Unprotected())
		r.Get("/embedded-shares", s.embedded.ListEmbeddedShares)
		r.Post("/process-embedded-share", s.embedded.ProcessEmbeddedShare)
	})
}
