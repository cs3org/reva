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
// written alongside them: the routes are read back out of the router, so an
// endpoint cannot be served without appearing here, nor appear here without
// being served.
package openapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/openapi"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the documentation is served: the document itself at
// <mount>.json, the page that browses it at <mount>.
const mount = "/openapi"

//go:embed ui.html
var uiPage string

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
	// Public serves the document without authentication. It is off by
	// default: the document is an inventory of everything the deployment
	// serves, which is not something to publish without meaning to. The page
	// that browses it is always public, as it holds nothing of the
	// deployment's own and asks for credentials when the document needs them.
	Public bool `mapstructure:"public"`
	// UIBundle is where the page loads its viewer from. It defaults to a
	// fixed release on a public CDN, and a deployment without egress should
	// point it at its own copy of swagger-ui-dist.
	UIBundle string `mapstructure:"ui_bundle"`
	// UIBundleIntegrity is the subresource integrity hash of UIBundle. The
	// viewer runs in the deployment's origin, next to the credentials of
	// whoever opens it, so the browser refuses a bundle that does not match.
	// It defaults to the hash of the default bundle, and only to that.
	UIBundleIntegrity string `mapstructure:"ui_bundle_integrity"`
	// UIStylesheet is the viewer's stylesheet, alongside UIBundle.
	UIStylesheet string `mapstructure:"ui_stylesheet"`
	// UIStylesheetIntegrity is the subresource integrity hash of
	// UIStylesheet, as UIBundleIntegrity is of UIBundle.
	UIStylesheetIntegrity string `mapstructure:"ui_stylesheet_integrity"`
}

// The viewer a page loads unless configured otherwise: an exact release, with
// the hashes of its files as published to npm.
const (
	defaultUIBundle              = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.1/swagger-ui-bundle.js"
	defaultUIBundleIntegrity     = "sha384-ZPehFMQommnnuaZ4rpxgkgTT2DKFVp4hZC/7pLit+9Lek9T1YGSo23eHFbvNkXkw"
	defaultUIStylesheet          = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.33.1/swagger-ui.css"
	defaultUIStylesheetIntegrity = "sha384-Ov4/wv3j2bmct8cDc5X4ngJZohVPzEmc6uDPH8WeljUxO5vtoykvMEfbu9Vh6RaW"
)

func (c *config) ApplyDefaults() {
	if c.Title == "" {
		c.Title = "reva"
	}
	// A hash is defaulted only with the file it belongs to: it would refuse
	// any other.
	if c.UIBundle == "" {
		c.UIBundle = defaultUIBundle
		c.UIBundleIntegrity = defaultUIBundleIntegrity
	}
	if c.UIStylesheet == "" {
		c.UIStylesheet = defaultUIStylesheet
		c.UIStylesheetIntegrity = defaultUIStylesheetIntegrity
	}
}

type svc struct {
	conf *config

	// routes is the table every service declared on. It is read when the
	// document is first asked for, not when this service is constructed:
	// the other services have not declared yet at that point.
	routes *router.Router

	// schemes is how this server authenticates, as its auth middleware was
	// configured. The document reports it so that a reader is told what to
	// send, rather than this service deciding for itself.
	schemes []auth.Scheme

	once sync.Once
	doc  []byte
	err  error
}

// New returns a new openapi service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &svc{conf: &c, schemes: auth.SchemesFromContext(ctx)}, nil
}

func (s *svc) Close() error { return nil }

// Routes declares the two endpoints, and keeps the router so the document can
// be built from what every service declared on it.
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

	// The page is public whatever the document is: a client that
	// authenticates with a token has no other place to enter it.
	r.Get(mount, s.page, router.Unprotected(), router.Describe(router.Doc{
		Summary: "Browse the API",
	}))
}

// document renders the OpenAPI document, once: the routes cannot change after
// the server has started.
func (s *svc) document(w http.ResponseWriter, r *http.Request) {
	s.once.Do(func() {
		var servers []openapi.Server
		if s.conf.URL != "" {
			servers = []openapi.Server{{URL: strings.TrimSuffix(s.conf.URL, "/")}}
		}

		doc := openapi.From(s.routes.Routes(), openapi.Info{
			Title:       s.conf.Title,
			Version:     rversion.Version,
			Description: s.conf.Description,
		}, servers, s.schemes)

		s.doc, s.err = json.MarshalIndent(doc, "", "  ")
	})

	if s.err != nil {
		appctx.GetLogger(r.Context()).Error().Err(s.err).Msg("openapi: rendering the document")
		http.Error(w, "could not render the document", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if _, err := w.Write(s.doc); err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("openapi: writing the document")
	}
}

// page serves the viewer, pointed at the document.
func (s *svc) page(w http.ResponseWriter, r *http.Request) {
	html := strings.NewReplacer(
		"{{.Title}}", template(s.conf.Title),
		"{{.Spec}}", template(mount+".json"),
		"{{.Bundle}}", template(s.conf.UIBundle),
		"{{.BundleIntegrity}}", template(s.conf.UIBundleIntegrity),
		"{{.Stylesheet}}", template(s.conf.UIStylesheet),
		"{{.StylesheetIntegrity}}", template(s.conf.UIStylesheetIntegrity),
	).Replace(uiPage)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("openapi: writing the page")
	}
}

// template escapes a configured value for the places the page puts it, all of
// which are quoted attributes or string literals.
func template(v string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;",
	).Replace(v)
}
