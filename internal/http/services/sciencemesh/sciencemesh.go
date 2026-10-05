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
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/ratelimit"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/smtpclient"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/go-chi/chi/v5"
)

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

	r := chi.NewRouter()
	s := &svc{
		conf:   &c,
		router: r,
	}

	if err := s.routerInit(); err != nil {
		_ = s.Close()
		return nil, err
	}

	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	s.limiter.Close()
	return nil
}

type config struct {
	Prefix               string                      `mapstructure:"prefix"`
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
	// AllowedFederationCIDRs is an explicit private-network exception list for
	// the public-only OCM discovery client. Empty by default; any invalid entry
	// aborts service initialization before any directory fetch.
	AllowedFederationCIDRs []string `mapstructure:"allowed_federation_cidrs"`
	// UnauthRateLimit is the sustained number of requests per minute allowed
	// per client on the unauthenticated routes (see Unprotected). 0 selects the
	// default; a negative value disables limiting.
	UnauthRateLimit      int `mapstructure:"unauth_rate_limit"`
	UnauthRateLimitBurst int `mapstructure:"unauth_rate_limit_burst"`
	// TrustedProxyCIDRs lists the reverse proxies whose X-Forwarded-For header
	// is trusted to identify the client. Without it, clients behind a proxy
	// all share the proxy's address and therefore a single rate limit bucket.
	// Any invalid entry aborts service initialization.
	TrustedProxyCIDRs []string `mapstructure:"trusted_proxy_cidrs"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "sciencemesh"
	}
	if c.OCMMountPoint == "" {
		c.OCMMountPoint = "/ocm"
	}
	if c.OCMClientTimeout == 0 {
		c.OCMClientTimeout = 10
	}
	if c.UnauthRateLimit == 0 {
		c.UnauthRateLimit = 30
	}
	if c.UnauthRateLimitBurst == 0 {
		c.UnauthRateLimitBurst = 10
	}

	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

// unauthRateLimiter builds the limiter guarding the unauthenticated routes.
// It returns a nil limiter (a no-op middleware) when limiting is disabled.
func (c *config) unauthRateLimiter() (*ratelimit.Limiter, error) {
	if c.UnauthRateLimit < 0 {
		return nil, nil
	}
	return ratelimit.New(ratelimit.Config{
		RequestsPerMinute: c.UnauthRateLimit,
		Burst:             c.UnauthRateLimitBurst,
		TrustedProxyCIDRs: c.TrustedProxyCIDRs,
	})
}

type svc struct {
	conf    *config
	router  chi.Router
	limiter *ratelimit.Limiter
}

func (s *svc) routerInit() error {
	// Build the limiter before any handler startup I/O, so that an invalid
	// trusted_proxy_cidrs entry fails fast, like allowed_federation_cidrs.
	limiter, err := s.conf.unauthRateLimiter()
	if err != nil {
		return err
	}
	s.limiter = limiter
	log := appctx.GetLogger(context.Background())
	if limiter == nil {
		log.Warn().Msg("Rate limiting of unauthenticated routes is disabled")
	} else {
		log.Info().
			Int("requests_per_minute", s.conf.UnauthRateLimit).
			Int("burst", s.conf.UnauthRateLimitBurst).
			Int("trusted_proxies", len(s.conf.TrustedProxyCIDRs)).
			Msg("Rate limiting unauthenticated routes")
	}
	unauth := limiter.Middleware

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

	s.router.Post("/generate-invite", tokenHandler.Generate)
	s.router.Get("/list-invite", tokenHandler.ListInvite)
	s.router.Post("/accept-invite", tokenHandler.AcceptInvite)
	s.router.Get("/find-accepted-users", tokenHandler.FindAccepted)
	s.router.Delete("/delete-accepted-user", tokenHandler.DeleteAccepted)
	s.router.Get("/list-providers", providersHandler.ListProviders)
	s.router.Post("/open-in-app", appsHandler.OpenInApp)
	// Unauthenticated routes (keep in sync with Unprotected) are rate limited.
	s.router.With(unauth).Get("/federations", wayfHandler.GetFederations)
	s.router.With(unauth).Post("/discover", wayfHandler.DiscoverProvider)
	s.router.Get("/embedded-shares", embeddedHandler.ListEmbeddedShares)
	s.router.Post("/process-embedded-share", embeddedHandler.ProcessEmbeddedShare)
	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{"/federations", "/discover"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		log.Debug().Str("path", r.URL.Path).Msg("sciencemesh routing")

		// unset raw path, otherwise chi uses it to route and then fails to match percent encoded path segments
		r.URL.RawPath = ""
		s.router.ServeHTTP(w, r)
	})
}
