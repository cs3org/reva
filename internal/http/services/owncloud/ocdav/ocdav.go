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

package ocdav

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/notification/notificationhelper"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/favorite"
	"github.com/cs3org/reva/pkg/storage/favorite/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
)

type ctxKey int

const name = "ocdav"

const (
	ctxKeyBaseURI ctxKey = iota
	ctxOCM10
)

var (
	errInvalidValue = errors.New("invalid value")

	nameRules = [...]nameRule{
		nameNotEmpty{},
		nameDoesNotContain{chars: "\f\r\n\\"},
	}
)

type nameRule interface {
	Test(name string) bool
}

type nameNotEmpty struct{}

func (r nameNotEmpty) Test(name string) bool {
	return len(strings.TrimSpace(name)) > 0
}

type nameDoesNotContain struct {
	chars string
}

func (r nameDoesNotContain) Test(name string) bool {
	return !strings.ContainsAny(name, r.chars)
}

func init() {
	rhttp.Register(name, New)
}

// Config holds the config options that need to be passed down to all ocdav handlers.
type Config struct {
	// FilesNamespace prefixes the namespace, optionally with user information.
	// Example: if FilesNamespace is /users/{{substr 0 1 .Username}}/{{.Username}}
	// and received path is /docs the internal path will be:
	// /users/<first char of username>/<username>/docs
	FilesNamespace string `mapstructure:"files_namespace"`
	// WebdavNamespace prefixes the namespace, optionally with user information.
	// Example: if WebdavNamespace is /users/{{substr 0 1 .Username}}/{{.Username}}
	// and received path is /docs the internal path will be:
	// /users/<first char of username>/<username>/docs
	WebdavNamespace string `mapstructure:"webdav_namespace"`
	OCMNamespace    string `mapstructure:"ocm_namespace"`
	GatewaySvc      string `mapstructure:"gatewaysvc"`
	Timeout         int64  `mapstructure:"timeout"`
	Insecure        bool   `mapstructure:"insecure" docs:"false;Whether to skip certificate checks when sending requests."`
	// If true, HTTP COPY will expect the HTTP-TPC (third-party copy) headers
	EnableHTTPTpc bool `mapstructure:"enable_http_tpc"`
	// The authentication scheme to use for the tpc push call when userinfo part is specified in the Destination header uri. Default value is 'bearer'.
	// Possible values:
	// "bearer"				results in header: Authorization: Bearer ...token...
	// "x-access-token":	results in header: X-Access-Token: ...token...
	HTTPTpcPushAuthHeader  string                            `mapstructure:"http_tpc_push_auth_header"`
	PublicURL              string                            `mapstructure:"public_url"`
	FavoriteStorageDriver  string                            `mapstructure:"favorite_storage_driver"`
	FavoriteStorageDrivers map[string]map[string]interface{} `mapstructure:"favorite_storage_drivers"`
	Notifications          map[string]interface{}            `mapstructure:"notifications" docs:"Settingsg for the Notification Helper"`
}

func (c *Config) ApplyDefaults() {
	// note: default c.Prefix is an empty string
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.FavoriteStorageDriver == "" {
		c.FavoriteStorageDriver = "memory"
	}

	if c.OCMNamespace == "" {
		c.OCMNamespace = "/ocm"
	}
}

type svc struct {
	c                  *Config
	webDavHandler      *WebDavHandler
	davHandler         *DavHandler
	favoritesManager   favorite.Manager
	client             *http.Client
	notificationHelper *notificationhelper.NotificationHelper
}

func getFavoritesManager(c *Config) (favorite.Manager, error) {
	if f, ok := registry.NewFuncs[c.FavoriteStorageDriver]; ok {
		return f(c.FavoriteStorageDrivers[c.FavoriteStorageDriver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.FavoriteStorageDriver)
}

// New returns a new ocdav.
func New(ctx context.Context, m map[string]interface{}) (rhttp.Service, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	fm, err := getFavoritesManager(&c)
	if err != nil {
		return nil, err
	}

	log := appctx.GetLogger(ctx)
	s := &svc{
		c:             &c,
		webDavHandler: new(WebDavHandler),
		davHandler:    new(DavHandler),
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(time.Duration(c.Timeout*int64(time.Second))),
			rhttp.Insecure(c.Insecure),
		),
		favoritesManager:   fm,
		notificationHelper: notificationhelper.New("ocdav", c.Notifications, log),
	}

	// initialize handlers and set default cigs
	if err := s.webDavHandler.init(c.WebdavNamespace, true); err != nil {
		return nil, err
	}
	if err := s.davHandler.init(&c); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *svc) Name() string {
	return name
}

func (s *svc) Close() error {
	s.notificationHelper.Stop()
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{"/status.php", "/remote.php/dav/public-files/", "/apps/files/", "/index.php/f/", "/index.php/s/", "/remote.php/dav/ocm/"}
}

func (s *svc) Register(r mux.Router) {
	r.Handle("/status.php", s.Handler())
	r.Handle("/webdav/*", s.Handler())
	r.Handle("/dav/*", s.Handler())
	r.Handle("/remote.php/*", s.Handler())
	r.Handle("/apps/*", s.Handler())
	r.Handle("/index.php/*", s.Handler())
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		addAccessHeaders(w, r)

		// TODO(jfd): do we need this?
		// fake litmus testing for empty namespace: see https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/litmus_test_server.go#L58-L89
		if r.Header.Get("X-Litmus") == "props: 3 (propfind_invalid2)" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		// to build correct href prop urls we need to keep track of the base path
		// always starts with /
		base := "/"

		var head string
		head, r.URL.Path = rhttp.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")
		switch head {
		case "status.php":
			s.doStatus(w, r)
			return
		case "remote.php":
			// skip optional "remote.php"
			head, r.URL.Path = rhttp.ShiftPath(r.URL.Path)

			// yet, add it to baseURI
			base = path.Join(base, "remote.php")
		case "apps":
			head, r.URL.Path = rhttp.ShiftPath(r.URL.Path)
			if head == "files" {
				s.handleLegacyPath(w, r)
				return
			}
		case "index.php":
			head, r.URL.Path = rhttp.ShiftPath(r.URL.Path)
			if head == "s" {
				token := r.URL.Path
				rURL := s.c.PublicURL + path.Join(head, token)

				http.Redirect(w, r, rURL, http.StatusMovedPermanently)
				return
			}
		}
		switch head {
		// the old `/webdav` endpoint uses remote.php/webdav/$path
		case "webdav":
			// for oc we need to prepend /home as the path that will be passed to the home storage provider
			// will not contain the username
			base = path.Join(base, "webdav")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			s.webDavHandler.Handler(s).ServeHTTP(w, r)
			return
		case "dav":
			// cern uses /dav/files/$namespace -> /$namespace/...
			// oc uses /dav/files/$user -> /$home/$user/...
			// for oc we need to prepend the path to user homes
			// or we take the path starting at /dav and allow rewriting it?
			base = path.Join(base, "dav")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			s.davHandler.Handler(s).ServeHTTP(w, r)
			return
		}
		log.Warn().Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
	})
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
}

func applyLayout(ctx context.Context, ns string, useLoggedInUserNS bool, requestPath string) string {
	// If useLoggedInUserNS is false, that implies that the request is coming from
	// the FilesHandler method invoked by a /dav/files/fileOwner where fileOwner
	// is not the same as the logged in user. In that case, we'll treat fileOwner
	// as the username whose files are to be accessed and use that in the
	// namespace template.
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok || !useLoggedInUserNS {
		requestUserID, _ := rhttp.ShiftPath(requestPath)
		u = &userpb.User{
			Username: requestUserID,
		}
	}
	return templates.WithUser(u, ns)
}

func addAccessHeaders(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	// the webdav api is accessible from anywhere
	headers.Set("Access-Control-Allow-Origin", "*")
	// all resources served via the DAV endpoint should have the strictest possible as default
	headers.Set("Content-Security-Policy", "default-src 'none';")
	// disable sniffing the content type for IE
	headers.Set("X-Content-Type-Options", "nosniff")
	// https://msdn.microsoft.com/en-us/library/jj542450(v=vs.85).aspx
	headers.Set("X-Download-Options", "noopen")
	// Disallow iFraming from other domains
	headers.Set("X-Frame-Options", "SAMEORIGIN")
	// https://www.adobe.com/devnet/adobe-media-server/articles/cross-domain-xml-for-streaming.html
	headers.Set("X-Permitted-Cross-Domain-Policies", "none")
	// https://developers.google.com/webmasters/control-crawl-index/docs/robots_meta_tag
	headers.Set("X-Robots-Tag", "none")
	// enforce browser based XSS filters
	headers.Set("X-XSS-Protection", "1; mode=block")

	if r.TLS != nil {
		headers.Set("Strict-Transport-Security", "max-age=63072000")
	}
}

func extractDestination(r *http.Request) (string, error) {
	dstHeader := r.Header.Get(HeaderDestination)
	if dstHeader == "" {
		return "", errors.Wrap(errInvalidValue, "destination header is empty")
	}
	dstURL, err := url.ParseRequestURI(dstHeader)
	if err != nil {
		return "", errors.Wrap(errInvalidValue, err.Error())
	}

	baseURI := r.Context().Value(ctxKeyBaseURI).(string)
	// TODO check if path is on same storage, return 502 on problems, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	// Strip the base URI from the destination. The destination might contain redirection prefixes which need to be handled
	urlSplit := strings.Split(dstURL.Path, baseURI)
	if len(urlSplit) != 2 {
		return "", errors.Wrap(errInvalidValue, "destination path does not contain base URI")
	}

	return urlSplit[1], nil
}

// replaceAllStringSubmatchFunc is taken from 'Go: Replace String with Regular Expression Callback'
// see: https://elliotchance.medium.com/go-replace-string-with-regular-expression-callback-f89948bad0bb
func replaceAllStringSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) string) string {
	result := ""
	lastIndex := 0
	for _, v := range re.FindAllSubmatchIndex([]byte(str), -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}
		result += str[lastIndex:v[0]] + repl(groups)
		lastIndex = v[1]
	}
	return result + str[lastIndex:]
}

var hrefre = regexp.MustCompile(`([^A-Za-z0-9_\-.~()/:@!$])`)

// encodePath encodes the path of a url.
//
// slashes (/) are treated as path-separators.
// ported from https://github.com/sabre-io/http/blob/bb27d1a8c92217b34e778ee09dcf79d9a2936e84/lib/functions.php#L369-L379
func encodePath(path string) string {
	return replaceAllStringSubmatchFunc(hrefre, path, func(groups []string) string {
		b := groups[1]
		var sb strings.Builder
		for i := 0; i < len(b); i++ {
			sb.WriteString(fmt.Sprintf("%%%x", b[i]))
		}
		return sb.String()
	})
}
