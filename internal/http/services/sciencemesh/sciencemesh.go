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

package sciencemesh

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/smtpclient"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const name = "sciencemesh"

func init() {
	rhttp.Register(name, New)
}

// New returns a new sciencemesh service.
func New(ctx context.Context, m map[string]interface{}) (rhttp.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		conf: &c,
	}

	if err := s.initHandlers(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) initHandlers() error {
	s.tokenHandler = new(tokenHandler)
	if err := s.tokenHandler.init(s.conf); err != nil {
		return err
	}
	s.providersHandler = new(providersHandler)
	if err := s.providersHandler.init(s.conf); err != nil {
		return err
	}
	s.sharesHandler = new(sharesHandler)
	if err := s.sharesHandler.init(s.conf); err != nil {
		return err
	}
	s.appsHandler = new(appsHandler)
	if err := s.appsHandler.init(s.conf); err != nil {
		return err
	}
	return nil
}

func (s *svc) Name() string {
	return name
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	SMTPCredentials  *smtpclient.SMTPCredentials `mapstructure:"smtp_credentials"   validate:"required"`
	GatewaySvc       string                      `mapstructure:"gatewaysvc"         validate:"required"`
	MeshDirectoryURL string                      `mapstructure:"mesh_directory_url" validate:"required"`
	ProviderDomain   string                      `mapstructure:"provider_domain"    validate:"required"`
	SubjectTemplate  string                      `mapstructure:"subject_template"`
	BodyTemplatePath string                      `mapstructure:"body_template_path"`
	OCMMountPoint    string                      `mapstructure:"ocm_mount_point"`
}

func (c *config) ApplyDefaults() {
	if c.OCMMountPoint == "" {
		c.OCMMountPoint = "/ocm"
	}
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	conf             *config
	tokenHandler     *tokenHandler
	providersHandler *providersHandler
	sharesHandler    *sharesHandler
	appsHandler      *appsHandler
}

func (s *svc) Register(r mux.Router) {
	r.Route("/sciencemesh", func(r mux.Router) {
		r.Get("/generate-invite", http.HandlerFunc(s.tokenHandler.Generate))
		r.Get("/list-invite", http.HandlerFunc(s.tokenHandler.ListInvite))
		r.Post("/accept-invite", http.HandlerFunc(s.tokenHandler.AcceptInvite))
		r.Get("/find-accepted-users", http.HandlerFunc(s.tokenHandler.FindAccepted))
		r.Delete("/delete-accepted-user", http.HandlerFunc(s.tokenHandler.DeleteAccepted))
		r.Get("/list-providers", http.HandlerFunc(s.providersHandler.ListProviders))
		r.Post("/create-share", http.HandlerFunc(s.sharesHandler.CreateShare))
		r.Post("/open-in-app", http.HandlerFunc(s.appsHandler.OpenInApp))
	})
}
