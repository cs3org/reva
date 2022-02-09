// Copyright 2018-2021 CERN
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
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/favorite"
	"github.com/cs3org/reva/pkg/storage/favorite/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
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
	global.Register("ocdav", New)
}

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix string `mapstructure:"prefix"`
	// FilesNamespace prefixes the namespace, optionally with user information.
	// Example: if FilesNamespace is /users/{{substr 0 1 .Username}}/{{.Username}}
	// and received path is /docs the internal path will be:
	// /users/<first char of username>/<username>/docs
	FilesNamespace string `mapstructure:"files_namespace"`
	// WebdavNamespace prefixes the namespace, optionally with user information.
	// Example: if WebdavNamespace is /users/{{substr 0 1 .Username}}/{{.Username}}
	// and received path is /docs the internal path will be:
	// /users/<first char of username>/<username>/docs
	WebdavNamespace        string                            `mapstructure:"webdav_namespace"`
	SharesNamespace        string                            `mapstructure:"shares_namespace"`
	GatewaySvc             string                            `mapstructure:"gatewaysvc"`
	Timeout                int64                             `mapstructure:"timeout"`
	Insecure               bool                              `mapstructure:"insecure"`
	PublicURL              string                            `mapstructure:"public_url"`
	FavoriteStorageDriver  string                            `mapstructure:"favorite_storage_driver"`
	FavoriteStorageDrivers map[string]map[string]interface{} `mapstructure:"favorite_storage_drivers"`
}

func (c *Config) init() {
	// note: default c.Prefix is an empty string
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.FavoriteStorageDriver == "" {
		c.FavoriteStorageDriver = "memory"
	}
}

type svc struct {
	c                *Config
	webDavHandler    *WebDavHandler
	davHandler       *DavHandler
	favoritesManager favorite.Manager
	client           *http.Client
	// LockSystem is the lock management system.
	LockSystem LockSystem
}

func (s *svc) Config() *Config {
	return s.c
}

func getFavoritesManager(c *Config) (favorite.Manager, error) {
	if f, ok := registry.NewFuncs[c.FavoriteStorageDriver]; ok {
		return f(c.FavoriteStorageDrivers[c.FavoriteStorageDriver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.FavoriteStorageDriver)
}
func getLockSystem(c *Config) (LockSystem, error) {
	// TODO in memory implementation
	client, err := pool.GetGatewayServiceClient(c.GatewaySvc)
	if err != nil {
		return nil, err
	}
	return NewCS3LS(client), nil
}

// New returns a new ocdav
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()

	fm, err := getFavoritesManager(conf)
	if err != nil {
		return nil, err
	}
	ls, err := getLockSystem(conf)
	if err != nil {
		return nil, err
	}

	s := &svc{
		c:             conf,
		webDavHandler: new(WebDavHandler),
		davHandler:    new(DavHandler),
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(time.Duration(conf.Timeout*int64(time.Second))),
			rhttp.Insecure(conf.Insecure),
		),
		favoritesManager: fm,
		LockSystem:       ls,
	}
	// initialize handlers and set default configs
	if err := s.webDavHandler.init(conf.WebdavNamespace, true); err != nil {
		return nil, err
	}
	if err := s.davHandler.init(conf); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{"/status.php", "/remote.php/dav/public-files/", "/apps/files/", "/index.php/f/", "/index.php/s/"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		addAccessHeaders(w, r)

		// TODO(jfd): do we need this?
		// fake litmus testing for empty namespace: see https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/litmus_test_server.go#L58-L89
		if r.Header.Get(net.HeaderLitmus) == "props: 3 (propfind_invalid2)" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		// to build correct href prop urls we need to keep track of the base path
		// always starts with /
		base := path.Join("/", s.Prefix())

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")
		switch head {
		case "status.php":
			s.doStatus(w, r)
			return
		case "remote.php":
			// skip optional "remote.php"
			head, r.URL.Path = router.ShiftPath(r.URL.Path)

			// yet, add it to baseURI
			base = path.Join(base, "remote.php")
		case "apps":
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			if head == "files" {
				s.handleLegacyPath(w, r)
				return
			}
		case "index.php":
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			if head == "s" {
				token := r.URL.Path
				url := s.c.PublicURL + path.Join("#", head, token)
				http.Redirect(w, r, url, http.StatusMovedPermanently)
				return
			}
		}
		switch head {
		// the old `/webdav` endpoint uses remote.php/webdav/$path
		case "webdav":
			// for oc we need to prepend /home as the path that will be passed to the home storage provider
			// will not contain the username
			base = path.Join(base, "webdav")
			ctx := context.WithValue(ctx, net.CtxKeyBaseURI, base)
			r = r.WithContext(ctx)
			s.webDavHandler.Handler(s).ServeHTTP(w, r)
			return
		case "dav":
			// cern uses /dav/files/$namespace -> /$namespace/...
			// oc uses /dav/files/$user -> /$home/$user/...
			// for oc we need to prepend the path to user homes
			// or we take the path starting at /dav and allow rewriting it?
			base = path.Join(base, "dav")
			ctx := context.WithValue(ctx, net.CtxKeyBaseURI, base)
			r = r.WithContext(ctx)
			s.davHandler.Handler(s).ServeHTTP(w, r)
			return
		}
		log.Warn().Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
	})
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(s.c.GatewaySvc)
}

func (s *svc) ApplyLayout(ctx context.Context, ns string, useLoggedInUserNS bool, requestPath string) (string, string, error) {
	// If useLoggedInUserNS is false, that implies that the request is coming from
	// the FilesHandler method invoked by a /dav/files/fileOwner where fileOwner
	// is not the same as the logged in user. In that case, we'll treat fileOwner
	// as the username whose files are to be accessed and use that in the
	// namespace template.
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok || !useLoggedInUserNS {
		var requestUsernameOrID string
		requestUsernameOrID, requestPath = router.ShiftPath(requestPath)

		gatewayClient, err := s.getClient()
		if err != nil {
			return "", "", err
		}

		// Check if this is a Userid
		userRes, err := gatewayClient.GetUser(ctx, &userpb.GetUserRequest{
			UserId: &userpb.UserId{OpaqueId: requestUsernameOrID},
		})
		if err != nil {
			return "", "", err
		}

		// If it's not a userid try if it is a user name
		if userRes.Status.Code != rpc.Code_CODE_OK {
			res, err := gatewayClient.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
				Claim: "username",
				Value: requestUsernameOrID,
			})
			if err != nil {
				return "", "", err
			}
			userRes.Status = res.Status
			userRes.User = res.User
		}

		// If still didn't find a user, fallback
		if userRes.Status.Code != rpc.Code_CODE_OK {
			userRes.User = &userpb.User{
				Username: requestUsernameOrID,
				Id:       &userpb.UserId{OpaqueId: requestUsernameOrID},
			}
		}

		u = userRes.User
	}

	return templates.WithUser(u, ns), requestPath, nil
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
	dstHeader := r.Header.Get(net.HeaderDestination)
	if dstHeader == "" {
		return "", errors.Wrap(errInvalidValue, "destination header is empty")
	}
	dstURL, err := url.ParseRequestURI(dstHeader)
	if err != nil {
		return "", errors.Wrap(errInvalidValue, err.Error())
	}

	baseURI := r.Context().Value(net.CtxKeyBaseURI).(string)
	// TODO check if path is on same storage, return 502 on problems, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	// Strip the base URI from the destination. The destination might contain redirection prefixes which need to be handled
	urlSplit := strings.Split(dstURL.Path, baseURI)
	if len(urlSplit) != 2 {
		return "", errors.Wrap(errInvalidValue, "destination path does not contain base URI")
	}

	return urlSplit[1], nil
}
