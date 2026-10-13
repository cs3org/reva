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

// Package mcp serves the files of the authenticated user to AI clients over
// the Model Context Protocol. It accepts only mcp app passwords, and offers
// the tools their permissions allow.
package mcp

import (
	"cmp"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func init() {
	global.Register("mcp", New)
}

type rateLimitConfig struct {
	CallsPerTokenPerMin      int `mapstructure:"calls_per_token_per_min" docs:"120;Tool calls a minute, per app password and, with the same value, per user."`
	WriteCallsPerTokenPerMin int `mapstructure:"write_calls_per_token_per_min" docs:"10;Calls a minute to the tools that change something, per app password and per user."`
	ReadConcurrencyPerToken  int `mapstructure:"read_concurrency_per_token" docs:"10;read_file calls in flight, per app password and per user."`
	ReadBytesPerTokenPerMin  int `mapstructure:"read_bytes_per_token_per_min" docs:"20971520;Bytes read_file may return a minute, per app password and per user."`
}

type config struct {
	Prefix         string          `mapstructure:"prefix" docs:"mcp;The URL path the service is served at."`
	WebBaseURL     string          `mapstructure:"web_base_url" docs:";Base URL of the web UI, for the web_url of every entry and the URL of public links."`
	WebURLTemplate string          `mapstructure:"web_url_template" docs:"{base}/files/spaces{path};The web_url of an entry; {base} is web_base_url, {path} the escaped path and {id} the escaped resource id."`
	MaxReadBytes   int64           `mapstructure:"max_read_bytes" docs:"1048576;Most bytes of text read_file returns in a call."`
	MaxImageBytes  int64           `mapstructure:"max_image_bytes" docs:"5242880;Largest image read_file returns."`
	MaxWriteBytes  int64           `mapstructure:"max_write_bytes" docs:"10485760;Largest content write_file accepts."`
	ListMaxEntries int             `mapstructure:"list_max_entries" docs:"500;Entries per page of a listing."`
	DisabledTools  []string        `mapstructure:"disabled_tools" docs:"nil;Tools not offered, as a kill switch."`
	RateLimit      rateLimitConfig `mapstructure:"ratelimit"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "mcp"
	}
	c.WebBaseURL = strings.TrimSuffix(c.WebBaseURL, "/")
	if c.WebURLTemplate == "" {
		c.WebURLTemplate = "{base}/files/spaces{path}"
	}
	if c.MaxReadBytes == 0 {
		c.MaxReadBytes = 1 << 20
	}
	if c.MaxImageBytes == 0 {
		c.MaxImageBytes = 5 << 20
	}
	if c.MaxWriteBytes == 0 {
		c.MaxWriteBytes = 10 << 20
	}
	if c.ListMaxEntries == 0 {
		c.ListMaxEntries = 500
	}
	if c.RateLimit.CallsPerTokenPerMin == 0 {
		c.RateLimit.CallsPerTokenPerMin = 120
	}
	if c.RateLimit.WriteCallsPerTokenPerMin == 0 {
		c.RateLimit.WriteCallsPerTokenPerMin = 10
	}
	if c.RateLimit.ReadConcurrencyPerToken == 0 {
		c.RateLimit.ReadConcurrencyPerToken = 10
	}
	if c.RateLimit.ReadBytesPerTokenPerMin == 0 {
		c.RateLimit.ReadBytesPerTokenPerMin = 20 << 20
	}
}

type svc struct {
	c       *config
	server  *sdk.Server
	tools   map[string]*toolInfo
	handler http.Handler
	limits  *limits
	client  *httpclient.Client
}

// New returns the MCP service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	s := newService(&c)
	appctx.GetLogger(ctx).Info().Str("service", "mcp").Str("prefix", c.Prefix).Int("tools", len(s.tools)).Msg("mcp service initialised")
	return s, nil
}

func newService(c *config) *svc {
	s := &svc{
		c:      c,
		limits: newLimits(),
		client: httpclient.New(httpclient.Timeout(5 * time.Minute)),
	}
	s.server = sdk.NewServer(&sdk.Implementation{Name: "cernbox", Title: "CERNBox", Version: cmp.Or(rversion.Version, "dev")}, &sdk.ServerOptions{
		Instructions: instructions,
		Capabilities: &sdk.ServerCapabilities{Tools: &sdk.ToolCapabilities{}},
	})
	s.registerTools()
	s.server.AddReceivingMiddleware(s.filterToolList)

	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return s.server }, &sdk.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
		// Behind the ingress, requests reach revad from a proxy on the same
		// host with the public host name. The bearer app password, which a
		// browser never sends on its own, already rules out DNS rebinding.
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        s.maxBodyBytes(),
	})
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The only credential the client holds is its app password.
		w.Header().Del(appctx.TokenHeader)
		if r.Method == http.MethodOptions {
			h.ServeHTTP(w, r)
			return
		}
		ap, ok := scope.AppAuthFromContext(r.Context())
		if !ok || ap.Client != scope.ClientMCP {
			http.Error(w, "an mcp app password is required", http.StatusForbidden)
			return
		}
		s.serveLimited(w, r, ap, h)
	})
	return s
}

// maxBodyBytes fits the largest write_file call: base64 content plus the
// JSON-RPC envelope.
func (s *svc) maxBodyBytes() int64 {
	return s.c.MaxWriteBytes/3*4 + 1<<20
}

func (s *svc) Handler() http.Handler { return s.handler }
func (s *svc) Prefix() string        { return s.c.Prefix }
func (s *svc) Close() error          { return nil }

// Unprotected is empty: the auth middleware guards everything.
func (s *svc) Unprotected() []string { return nil }

const instructions = `Tools for the user's CERNBox files. Paths are absolute CERNBox paths, such as /eos/user/j/jdoe/notes.txt or /eos/project/c/cernbox/docs; never relative.
Start with list_spaces to find the user's home and project spaces, then browse with list_folder. Entries carry a web_url: cite it so the user can open the file.
This connection may be restricted to some operations and to some folders. Tools that are not offered are not allowed; a not_permitted_by_app_password or outside_app_password_roots error means the user did not grant that, and retrying will not help.`
