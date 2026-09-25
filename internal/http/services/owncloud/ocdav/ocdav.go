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

package ocdav

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/v3/pkg/myofficefiles"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

type ctxKey int

const (
	ctxKeyBaseURI ctxKey = iota
	ctxSpaceID
	ctxSpacePath
	ctxOCM
	ctxPublicLink
	ctxStorageId
	ctxResourceOpaqueId
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

// mount is empty: the service claims several disjoint entry points rather than
// one subtree, and declares each of them in Routes.
const mount = ""

func init() {
	global.Register("ocdav", New)
}

type ConfigPublicLinkDownload struct {
	MaxNumFiles  int64  `mapstructure:"max_num_files"`
	MaxSize      int64  `mapstructure:"max_size"`
	PublicFolder string `mapstructure:"public_folder"`
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
	Insecure        bool   `docs:"false;Whether to skip certificate checks when sending requests." mapstructure:"insecure"`
	// If true, HTTP COPY will expect the HTTP-TPC (third-party copy) headers
	EnableHTTPTpc bool `mapstructure:"enable_http_tpc"`
	// The authentication scheme to use for the tpc push call when userinfo part is specified in the Destination header uri. Default value is 'bearer'.
	// Possible values:
	// "bearer"				results in header: Authorization: Bearer ...token...
	// "x-access-token":	results in header: X-Access-Token: ...token...
	HTTPTpcPushAuthHeader        string                    `mapstructure:"http_tpc_push_auth_header"`
	PublicURL                    string                    `mapstructure:"public_url"`
	PublicLinkDownload           *ConfigPublicLinkDownload `mapstructure:"publiclink_download"`
	DisabledOpenInAppPaths       []string                  `mapstructure:"disabled_open_in_app_paths"`
	MyOfficeFilesAllowedProjects []string                  `mapstructure:"my_office_files_projects"`
}

func (c *Config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.OCMNamespace == "" {
		c.OCMNamespace = "/ocm"
	}

	if len(c.MyOfficeFilesAllowedProjects) == 0 {
		c.MyOfficeFilesAllowedProjects = []string{"cernbox"}
	}
}

type svc struct {
	c                    *Config
	webDavHandler        *WebDavHandler
	davHandler           *DavHandler
	myOfficeFilesManager myofficefiles.Manager
	client               *httpclient.Client
}

// New returns a new ocdav.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	myOfficeFilesManager, err := myofficefiles.New(ctx, c.GatewaySvc, c.MyOfficeFilesAllowedProjects)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure}}

	s := &svc{
		c:             &c,
		webDavHandler: new(WebDavHandler),
		davHandler:    new(DavHandler),
		client: httpclient.New(
			httpclient.Timeout(time.Duration(c.Timeout*int64(time.Second))),
			httpclient.RoundTripper(tr),
		),
		myOfficeFilesManager: myOfficeFilesManager,
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

func (s *svc) Close() error {
	return nil
}

// Routes declares the URLs ocdav serves. The service answers a handful of
// disjoint entry points rather than one subtree, and each of the WebDAV ones
// is reachable both directly and under /remote.php, which older clients use.
//
// A route's static part is also the base URI the handler echoes back in href
// properties, so declaring the routes fixes the bases too: they no longer have
// to be accumulated segment by segment while the request is dispatched.
func (s *svc) Routes(rt *router.Router) {
	r := rt.Use(s.preamble)

	r.Get("/status.php", s.doStatus, router.Unprotected())
	r.Any("/s/{token}/download", s.handleLegacyPublicLinkDownload, router.Unprotected())
	r.Any("/apps/files/{path...}", s.handleLegacyPath, router.Unprotected())
	r.Get("/index.php/s/{token}", s.redirectPublicLink, router.Unprotected())
	r.Get("/ocm-provider", s.redirectOCMDiscovery, router.Unprotected())

	// The WebDAV subtrees are mounted rather than matched against patterns:
	// below each of them the path is a resource path, which has to reach the
	// handler exactly as the client sent it. A pattern match would canonicalize
	// it - clients do send "." segments - and redirect instead of serving.
	//
	// Each mount takes the one parameter its URL carries out of the path
	// itself, which is all that is left of what used to be a dispatch tree.
	for _, prefix := range []string{"", "/remote.php"} {
		webdav, dav := prefix+"/webdav", prefix+"/dav"

		r.Mount(webdav, s.webdav(webdav))
		r.Mount(dav+"/avatars", s.avatars(dav))
		r.Mount(dav+"/files", s.files(dav+"/files"))
		r.Mount(dav+"/meta", s.versions(dav+"/meta"))
		r.Mount(dav+"/trash-bin", s.trashbin(dav+"/trash-bin"))
		// The spaces trash bin reports hrefs under /spaces, not under itself,
		// and is matched ahead of /spaces because a longer mount wins.
		r.Mount(dav+"/spaces/trash-bin", s.spacesTrashbin(dav+"/spaces"))
		r.Mount(dav+"/spaces", s.spaces(dav+"/spaces"))

		// OCM and public links carry their own credentials, in the path or in
		// a header, so they are reachable without the auth middleware.
		r.Mount(dav+"/ocm", s.ocm(dav+"/ocm"), router.Unprotected())
		r.Mount(dav+"/public-files", s.publicFiles(dav+"/public-files"), router.Unprotected())
	}
}

// preamble is the work every ocdav request needs before it reaches a handler.
func (s *svc) preamble(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addAccessHeaders(w, r)

		// TODO(jfd): do we need this?
		// fake litmus testing for empty namespace: see https://github.com/golang/net/blob/e514e69ffb8bc3c76a71ae40de0118d794855992/webdav/litmus_test_server.go#L58-L89
		if r.Header.Get("X-Litmus") == "props: 3 (propfind_invalid2)" {
			http.Error(w, "400 Bad Request", http.StatusBadRequest)
			return
		}

		// Handlers echo the URL the client used back in href properties.
		ctx := context.WithValue(r.Context(), ctxKeyIncomingURL, r.URL.Path)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// below returns the request path under prefix, rooted, as the WebDAV handlers
// expect to receive it.
func below(r *http.Request, prefix string) string {
	p := strings.TrimPrefix(r.URL.Path, prefix)
	if p == "" || p[0] != '/' {
		return "/" + p
	}
	return p
}

// serveAt hands the request to h with its path reduced to what lies below
// prefix, and with base recorded as the URI the request was reached under.
func serveAt(w http.ResponseWriter, r *http.Request, base, prefix string, h http.Handler) {
	r.URL.Path = below(r, prefix)
	h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyBaseURI, base)))
}

func (s *svc) redirectPublicLink(w http.ResponseWriter, r *http.Request) {
	target := s.c.PublicURL + path.Join("s", r.PathValue("token"))
	r.URL.Path = "/" // reset old path for redirection
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// redirectOCMDiscovery supports the legacy OCM discovery endpoint.
func (s *svc) redirectOCMDiscovery(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/.well-known/ocm", http.StatusMovedPermanently)
}

// webdav serves the old endpoint, which addresses the user's home directly.
func (s *svc) webdav(prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveAt(w, r, prefix, prefix, s.webDavHandler.Handler(s))
	}
}

func applyLayout(ctx context.Context, ns string, useLoggedInUserNS bool, requestPath string) string {
	return ns
	// If useLoggedInUserNS is false, that implies that the request is coming from
	// the FilesHandler method invoked by a /dav/files/fileOwner where fileOwner
	// is not the same as the logged in user. In that case, we'll treat fileOwner
	// as the username whose files are to be accessed and use that in the
	// namespace template.
	/*
		u, ok := appctx.ContextGetUser(ctx)
		if !ok || !useLoggedInUserNS {
			requestUserID, _ := router.ShiftPath(requestPath)
			u = &userpb.User{
				Username: requestUserID,
			}
		}
		return templates.WithUser(u, ns)
	*/
}

func addAccessHeaders(w http.ResponseWriter, r *http.Request) {
	headers := w.Header()
	// the webdav api is accessible from anywhere
	headers.Set(HeaderAccessControlAllowOrigin, "*")
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

func extractDestination(r *http.Request, ns string) (string, error) {
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
	destination := strings.TrimPrefix(dstURL.Path, baseURI)

	// If the destination is in a spaces format, we replace with the space path
	dstSpaceID, dstRelPath := router.ShiftPath(destination)
	_, spaceRoot, ok := spaces.DecodeStorageSpaceIDToPath(dstSpaceID)
	if ok && ns != "/public" {
		destination = path.Join(spaceRoot, dstRelPath)
	} else {
		// If it is non-spaces, we join the namespace
		destination = path.Join(ns, destination)
	}

	return destination, nil
}

// replaceAllStringSubmatchFunc is taken from 'Go: Replace String with Regular Expression Callback'
// see: https://elliotchance.medium.com/go-replace-string-with-regular-expression-callback-f89948bad0bb
func replaceAllStringSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) string) string {
	var result strings.Builder
	lastIndex := 0
	for _, v := range re.FindAllStringSubmatchIndex(str, -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}
		result.WriteString(str[lastIndex:v[0]] + repl(groups))
		lastIndex = v[1]
	}
	return result.String() + str[lastIndex:]
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

func (s *svc) lookUpStorageSpaceReference(ctx context.Context, spaceID string, relativePath string) (*storageProvider.Reference, *rpc.Status, error) {
	// Get the getway client
	gatewayClient, err := service.Gateway(ctx)
	if err != nil {
		return nil, nil, err
	}

	// retrieve a specific storage space
	lSSReq := &storageProvider.ListStorageSpacesRequest{
		Filters: []*storageProvider.ListStorageSpacesRequest_Filter{
			{
				Type: storageProvider.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &storageProvider.ListStorageSpacesRequest_Filter_Id{
					Id: &storageProvider.StorageSpaceId{
						OpaqueId: spaceID,
					},
				},
			},
		},
	}

	lSSRes, err := gatewayClient.ListStorageSpaces(ctx, lSSReq)
	if err != nil || lSSRes.Status.Code != rpc.Code_CODE_OK {
		return nil, lSSRes.Status, err
	}

	if len(lSSRes.StorageSpaces) != 1 {
		return nil, nil, fmt.Errorf("unexpected number of spaces")
	}
	space := lSSRes.StorageSpaces[0]

	return &storageProvider.Reference{
		ResourceId: space.Root,
		Path:       utils.MakeRelativePath(relativePath),
	}, lSSRes.Status, nil
}

func requestWasMadeToResourceId(ctx context.Context, fn string) (ref *provider.Reference, ok bool) {
	if opaqueId := ctx.Value(ctxResourceOpaqueId); opaqueId != nil {
		storageId := ctx.Value(ctxStorageId)
		if storageId != nil {
			ref := &provider.Reference{
				// We make the path relative
				Path: path.Join(".", fn),
				ResourceId: &provider.ResourceId{
					StorageId: storageId.(string),
					OpaqueId:  opaqueId.(string),
				},
			}
			return ref, true
		}
	}
	return nil, false
}
