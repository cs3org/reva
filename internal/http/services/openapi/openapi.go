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

// Package openapi serves the HTTP surface of a reva deployment as an OpenAPI
// document, and a page that browses it.
//
// It describes what the services actually declared rather than a document
// written alongside them. Every HTTP service in the fleet answers for its own
// routes through the Admin API. Without the Admin API, the document describes
// the routes of this server only.
package openapi

import (
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/openapi"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/getkin/kin-openapi/openapi3"
)

// mount is where the documentation is served: the document itself at
// <mount>.json, the page that browses it at <mount>.
const mount = "/openapi"

// The viewer is an exact version, because its hash changes with every
// release.
const (
	defaultUIBundle              = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.1/swagger-ui-bundle.js"
	defaultUIBundleIntegrity     = "sha384-ZPehFMQommnnuaZ4rpxgkgTT2DKFVp4hZC/7pLit+9Lek9T1YGSo23eHFbvNkXkw"
	defaultUIStylesheet          = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.1/swagger-ui.css"
	defaultUIStylesheetIntegrity = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
)

//go:embed ui.html
var uiPage string

var page = template.Must(template.New("ui").Parse(uiPage))

func init() {
	global.Register("openapi", New)
}

type config struct {
	// Title names the deployment in the document.
	Title string `mapstructure:"title"`
	// Description is the prose shown above the endpoint list.
	Description string `mapstructure:"description"`
	// URL is the deployment's external base URL, which the page uses as the
	// server to send requests to. Empty leaves it to the browser's origin.
	URL string `mapstructure:"url"`
	// Public serves the documentation without authentication. It is off by
	// default: the document is an inventory of everything the deployment
	// serves, which is not something to publish without meaning to.
	Public bool `mapstructure:"public"`
	// RefreshInterval is how long a document is served before the next
	// request builds it again, to pick up services that came or went.
	RefreshInterval string `mapstructure:"refresh_interval"`
	// UIBundle is where the page loads its viewer from. It defaults to a
	// public CDN, and a deployment without egress should point it at its own
	// copy of swagger-ui-dist.
	UIBundle string `mapstructure:"ui_bundle"`
	// UIBundleIntegrity is the Subresource Integrity hash the browser checks
	// UIBundle against. The viewer runs with the session of whoever opens the
	// page, so a changed copy must not run. Empty skips the check.
	UIBundleIntegrity string `mapstructure:"ui_bundle_integrity"`
	// UIStylesheet is the viewer's stylesheet, alongside UIBundle.
	UIStylesheet string `mapstructure:"ui_stylesheet"`
	// UIStylesheetIntegrity is the hash UIStylesheet is checked against.
	UIStylesheetIntegrity string `mapstructure:"ui_stylesheet_integrity"`
}

func (c *config) ApplyDefaults() {
	if c.Title == "" {
		c.Title = "reva"
	}
	if c.RefreshInterval == "" {
		c.RefreshInterval = "1m"
	}
	// A default hash only fits the default file.
	if c.UIBundle == "" {
		c.UIBundle = defaultUIBundle
		c.UIBundleIntegrity = cmp.Or(c.UIBundleIntegrity, defaultUIBundleIntegrity)
	}
	if c.UIStylesheet == "" {
		c.UIStylesheet = defaultUIStylesheet
		c.UIStylesheetIntegrity = cmp.Or(c.UIStylesheetIntegrity, defaultUIStylesheetIntegrity)
	}
}

type svc struct {
	conf    *config
	refresh time.Duration
	fleet   fleet

	// routes and schemes describe this server, for when the fleet cannot be
	// asked.
	routes  *router.Router
	schemes []auth.Scheme

	mu    sync.Mutex
	doc   []byte
	built time.Time
}

// New returns a new openapi service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	refresh, err := time.ParseDuration(c.RefreshInterval)
	if err != nil {
		return nil, err
	}
	return &svc{
		conf:    &c,
		refresh: refresh,
		fleet:   adminFleet{},
		schemes: auth.SchemesFromContext(ctx),
	}, nil
}

func (s *svc) Close() error { return nil }

func (s *svc) Routes(r *router.Router) {
	s.routes = r

	var opts []router.Option
	if s.conf.Public {
		opts = append(opts, router.Unprotected())
	}

	r.Get(mount+".json", s.document, append(opts, router.Describe(router.Doc{
		Summary:     "The OpenAPI document describing this deployment",
		Description: "Built from the routes the services declared, so it cannot drift from what is served.",
	}))...)

	r.Get(mount, s.page, append(opts, router.Describe(router.Doc{
		Summary: "Browse the API",
	}))...)
}

func (s *svc) document(w http.ResponseWriter, r *http.Request) {
	doc, err := s.current(r.Context())
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("openapi: rendering the document")
		http.Error(w, "could not render the document", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if _, err := w.Write(doc); err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("openapi: writing the document")
	}
}

// current returns the document, and builds it again when it is older than the
// refresh interval. A build asks each service through the Admin API, which
// audits each call, so it happens only on request and not on a timer.
func (s *svc) current(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.doc != nil && time.Since(s.built) < s.refresh {
		return s.doc, nil
	}

	doc, err := s.build(ctx)
	if err == nil {
		var encoded []byte
		if encoded, err = json.MarshalIndent(doc, "", "  "); err == nil {
			s.doc, s.built = encoded, time.Now()
			return s.doc, nil
		}
	}
	if s.doc != nil {
		appctx.GetLogger(ctx).Warn().Err(err).Msg("openapi: serving the last document built")
		return s.doc, nil
	}
	return nil, err
}

func (s *svc) build(ctx context.Context) (*openapi3.T, error) {
	info := &openapi3.Info{
		Title:       s.conf.Title,
		Version:     cmp.Or(rversion.Version, "unknown"),
		Description: s.conf.Description,
	}
	doc := &openapi3.T{OpenAPI: openapi.Version, Info: info, Paths: openapi3.NewPaths()}
	if s.conf.URL != "" {
		doc.Servers = openapi3.Servers{{URL: strings.TrimSuffix(s.conf.URL, "/")}}
	}

	log := appctx.GetLogger(ctx)
	documents, err := s.fleet.Documents(ctx)
	if err != nil || len(documents) == 0 {
		log.Debug().Err(err).Msg("openapi: the fleet cannot be asked, describing this server only")
		local, err := openapi.From(s.routes.Routes(), info, s.schemes)
		if err != nil {
			return nil, err
		}
		openapi.Merge(doc, local)
		return doc, nil
	}

	// Sorted, so that the version of a schema that is kept does not depend on
	// the order in which the fleet answered.
	names := make([]string, 0, len(documents))
	for name := range documents {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		part, err := openapi3.NewLoader().LoadFromData(documents[name])
		if err != nil {
			log.Warn().Err(err).Str("service", name).Msg("openapi: leaving out a document that does not load")
			continue
		}
		openapi.Merge(doc, part)
	}
	return doc, nil
}

func (s *svc) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := page.Execute(w, map[string]string{
		"Title": s.conf.Title,
		// Relative, so the page also works behind a proxy that serves it
		// below a prefix.
		"Spec":                strings.TrimPrefix(mount, "/") + ".json",
		"Bundle":              s.conf.UIBundle,
		"BundleIntegrity":     s.conf.UIBundleIntegrity,
		"Stylesheet":          s.conf.UIStylesheet,
		"StylesheetIntegrity": s.conf.UIStylesheetIntegrity,
	})
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("openapi: writing the page")
	}
}
