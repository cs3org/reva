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

package ocdav

import (
	"context"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"google.golang.org/grpc/metadata"
)

// segment splits the first path segment off a rooted path, cleaning it first.
// It is how a mounted subtree reads the one parameter its URL carries, now
// that the router rather than the handler decides which subtree a request
// belongs to: what is left is reading a parameter, not deciding a route.
func segment(p string) (head, rest string) {
	return router.ShiftPath(p)
}

// avatars serves the placeholder avatar. The user segment is read but unused:
// there is no per-user avatar to serve yet.
func (s *svc) avatars(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			// No need for the user, and we need to answer preflight checks,
			// which carry no auth headers.
			r.URL.Path = "/"
			s.handleOptions(w, r)
			return
		}
		_, rest := segment(below(r, base+"/avatars"))
		r.URL.Path = rest
		ctx := context.WithValue(r.Context(), ctxKeyBaseURI, base)
		s.davHandler.AvatarsHandler.Handler(s).ServeHTTP(w, r.WithContext(ctx))
	}
}

// filesRoot answers /dav/files, which names no user. Listing it is disabled,
// so a client that did not ask for a depth gets told so; one that did is
// served its own files.
func (s *svc) filesNoUser(base string, w http.ResponseWriter, r *http.Request) {
	{
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		user, ok := appctx.ContextGetUser(ctx)
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Header.Get("Depth") == "" {
			log.Debug().Str("path", r.URL.Path).Msg("method not allowed")
			w.WriteHeader(http.StatusMethodNotAllowed)
			b, err := Marshal(exception{
				code:    SabredavMethodNotAllowed,
				message: "Listing members of this collection is disabled",
			}, ErrListingMembers)
			if err != nil {
				log.Error().Msgf("error marshaling xml response: %s", b)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if _, err = w.Write(b); err != nil {
				log.Error().Msgf("error writing xml response: %s", b)
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		r.URL.Path = "/"
		r = r.WithContext(context.WithValue(ctx, ctxKeyBaseURI, path.Join(base, user.Username)))
		s.handleFilesRoot(w, r)
	}
}

// filesUserRoot lists the syncable roots of a user, so a sync client can pick
// what to sync.
func (s *svc) filesUserRoot(base, user string, w http.ResponseWriter, r *http.Request) {
	r.URL.Path = "/"
	ctx := context.WithValue(r.Context(), ctxKeyBaseURI, path.Join(base, user))
	s.handleFilesRoot(w, r.WithContext(ctx))
}

// filesHome serves the user's home. It is the one root that is not a literal
// CS3 path: homes are sharded per user, so the home has to be resolved.
func (s *svc) filesHome(base, user, rest string, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	client, err := service.Gateway(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res, err := client.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error getting user home")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(log, w, res.Status)
		return
	}

	r.URL.Path = path.Join(res.Path, rest)
	ctx = context.WithValue(ctx, ctxKeyBaseURI, path.Join(base, user))
	s.davHandler.FilesHomeHandler.Handler(s).ServeHTTP(w, r.WithContext(ctx))
}

// files serves the dav-files endpoint, a thin view over the CS3 namespace. The
// first segment names the user; the next names a top-level root, which is a
// literal CS3 path except for "home", and nothing at all means the list of
// roots a sync client can choose from.
func (s *svc) files(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := below(r, base)
		if p == "/" {
			s.filesNoUser(base, w, r)
			return
		}

		user, rest := segment(p)
		root, below := segment(rest)
		switch {
		case rest == "/":
			s.filesUserRoot(base, user, w, r)
		case root == "home":
			s.filesHome(base, user, below, w, r)
		default:
			r.URL.Path = rest
			ctx := context.WithValue(r.Context(), ctxKeyBaseURI, path.Join(base, user))
			s.davHandler.FilesHandler.Handler(s).ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// versions serves the versions of a resource, addressed by its id.
func (s *svc) versions(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, rest := segment(below(r, base))
		kind, key := segment(rest)
		if kind != "v" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		rid, ok := spaces.ParseResourceID(id)
		if !ok {
			// If this fails, the client might be non-spaces.
			var err error
			rid, err = spaces.ResourceIdFromString(id)
			if err != nil {
				http.Error(w, "400 Bad Request", http.StatusBadRequest)
				return
			}
		}

		r.URL.Path = key
		ctx := context.WithValue(r.Context(), ctxKeyBaseURI, base)
		version, _ := segment(key)
		s.davHandler.MetaHandler.VersionsHandler.Handler(s, rid, version).ServeHTTP(w, r.WithContext(ctx))
	}
}

// trashbin serves a user's trash bin.
func (s *svc) trashbin(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveAt(w, r, base, base, s.davHandler.TrashbinHandler.Handler(s))
	}
}

// spacesTrashbin serves the trash bin of a space. Its hrefs are reported under
// /spaces rather than under the trash bin itself.
func (s *svc) spacesTrashbin(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveAt(w, r, base, base+"/trash-bin", s.davHandler.TrashbinHandler.HandlerSpaces(s))
	}
}

// spaces serves a resource addressed by a space id, or by a resource id for
// the methods that accept one. Either way the id is resolved to a path, which
// is what the WebDAV handlers below work on.
func (s *svc) spaces(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		head, rest := segment(below(r, base))

		_, spacePath, ok := spaces.DecodeStorageSpaceIDToPath(head)
		switch {
		case ok:
			// The id names a space: replace it with the path the space lives
			// at, so the existing WebDAV handlers can serve it.
			ctx = context.WithValue(ctx, ctxSpaceID, head)
			r.URL.Path = filepath.Join(spacePath, rest)
		case r.Method == http.MethodPut || r.Method == MethodPropfind:
			// PUT and PROPFIND also accept a resource id, e.g. eos$space!inode.
			storageID, base, itemID, decoded := spaces.DecodeToResourceID(head)
			if !decoded {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			spacePath = base
			ctx = context.WithValue(ctx, ctxSpaceID, spaces.EncodeSpaceID(base))
			ctx = context.WithValue(ctx, ctxStorageId, storageID)
			ctx = context.WithValue(ctx, ctxResourceOpaqueId, itemID)
			r.URL.Path = rest
		default:
			// Neither a space nor, for this method, a resource id. Rejecting
			// it keeps an empty base from resolving to "/", which would make
			// COPY and MOVE act on the storage root.
			log.Warn().Str("head", head).Str("method", r.Method).Msg("spaces: head is not a valid space or resource ID")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		ctx = context.WithValue(ctx, ctxSpacePath, spacePath)
		ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
		s.davHandler.SpacesHandler.Handler(s).ServeHTTP(w, r.WithContext(ctx))
	}
}

// ocm serves a share received from another provider. The request carries its
// own credentials, either an exchanged JWT or the legacy shared secret, so it
// is authenticated here rather than by the auth middleware.
func (s *svc) ocm(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		c, err := service.Gateway(ctx)
		if err != nil {
			log.Error().Err(err).Msg("error getting gateway during OCM authentication")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var token, ocmshare, authType, mode string
		var relPath string
		if strings.Contains(r.Header.Get("Authorization"), "Bearer") {
			// OCM v1.1+ (OCIS et al.). The bearer is either the exchanged JWT
			// or the legacy shared secret; the path is /{shareId}/path.
			token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			ocmshare, relPath = segment(below(r, base))
			if isJWT(token) {
				authType = "ocmexchangedtoken"
			} else {
				authType = "ocmshares"
			}
			mode = "bearer"
		} else if username, _, ok := r.BasicAuth(); ok {
			// OCM v1.0 (OC10 and Nextcloud) carries the shared secret in basic
			// auth and does not pass the shareId, so what the route matched as
			// the share is the first segment of the path.
			token = username
			relPath = strings.TrimPrefix(below(r, base), "/")
			authType = "ocmshares"
			mode = "legacy"
		} else {
			log.Info().Any("url", r.URL.Path).Any("headers", r.Header).Msg("unauthenticated remote OCM access")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		authRes, err := handleOCMAuth(ctx, c, ocmshare, token, authType)
		switch {
		case err != nil:
			log.Info().Err(err).Str("mode", mode).Msg("error authenticating remote OCM access")
			w.WriteHeader(http.StatusInternalServerError)
			return
		case authRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
			log.Info().Str("token", token).Str("mode", mode).Msg("permission denied in remote OCM access")
			w.WriteHeader(http.StatusUnauthorized)
			return
		case authRes.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
			log.Info().Str("token", token).Str("mode", mode).Msg("unauthorized token in remote OCM access")
			w.WriteHeader(http.StatusUnauthorized)
			return
		case authRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
			log.Info().Str("token", token).Str("mode", mode).Msg("invalid token in remote OCM access")
			w.WriteHeader(http.StatusUnauthorized)
			return
		case authRes.Status.Code != rpc.Code_CODE_OK:
			log.Error().Str("token", token).Str("mode", mode).Interface("status", authRes.Status).Msg("grpc auth request failed in remote OCM access")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Basic auth does not carry the shareId, so recover the canonical one
		// through a token lookup.
		if mode == "legacy" && ocmshare == "" {
			shareRes, shareErr := c.GetOCMShareByToken(ctx, &ocmv1beta1.GetOCMShareByTokenRequest{Token: token})
			if shareErr == nil && shareRes.GetStatus().GetCode() == rpc.Code_CODE_OK {
				ocmshare = shareRes.GetShare().GetId().GetOpaqueId()
			}
		}

		internalPath, updateIncomingURL := ocmInternalPath(authType, token, ocmshare, relPath)
		r.URL.Path = internalPath
		if updateIncomingURL {
			ctx = context.WithValue(ctx, ctxKeyIncomingURL, internalPath)
		}

		ctx = appctx.ContextSetToken(ctx, authRes.Token)
		ctx = appctx.ContextSetUser(ctx, authRes.User)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)
		ctx = context.WithValue(ctx, ctxOCM, true)
		ctx = context.WithValue(ctx, ctxKeyBaseURI, base)

		log.Info().Str("token", token).Str("mode", mode).Str("ocmshare", ocmshare).Interface("user", authRes.User).Msg("remote OCM access authenticated")

		s.davHandler.OCMSharesHandler.Handler(s).ServeHTTP(w, r.WithContext(ctx))
	}
}

// publicFiles serves a public link. The link token is the credential, so the
// request authenticates itself here.
func (s *svc) publicFiles(base string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		c, err := service.Gateway(ctx)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		r.URL.Path = below(r, base)
		token, _ := segment(r.URL.Path)
		ctx = context.WithValue(ctx, ctxPublicLink, token)
		ctx = context.WithValue(ctx, ctxKeyBaseURI, base)

		res, hasValidBasicAuthHeader, unauthorized, err := authenticatePublicFilesRequest(ctx, r, c, token)
		if unauthorized {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case err != nil:
			log.Error().Str("token", token).Err(err).Msg("Error while handling public-files DAV request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		case res.Status == nil:
			log.Error().Msg("DAV public-files got a AuthenticateResponse without status!")
			w.WriteHeader(http.StatusInternalServerError)
			return
		case res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED, res.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
			w.WriteHeader(http.StatusUnauthorized)
			if hasValidBasicAuthHeader {
				b, err := Marshal(exception{
					code:    SabredavNotAuthenticated,
					message: "Username or password was incorrect",
				}, ErrInvalidCredentials)
				HandleWebdavError(log, w, b, err)
				return
			}
			b, err := Marshal(exception{
				code:    SabredavNotAuthenticated,
				message: "No 'Authorization: Basic' header found",
			}, ErrMissingBasicAuth)
			HandleWebdavError(log, w, b, err)
			return
		case res.Status.Code == rpc.Code_CODE_NOT_FOUND:
			w.WriteHeader(http.StatusNotFound)
			return
		case res.Status.Code != rpc.Code_CODE_OK:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		ctx = appctx.ContextSetToken(ctx, res.Token)
		ctx = appctx.ContextSetUser(ctx, res.User)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, res.Token)
		r = r.WithContext(ctx)

		// The public share manager knew the token, but does the referenced
		// target still exist?
		sRes, err := getTokenStatInfo(ctx, c, token)
		switch {
		case err != nil:
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		case sRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED, sRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("token", token).Interface("status", sRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
			return
		case sRes.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
			log.Debug().Str("token", token).Interface("status", sRes.Status).Msg("unauthorized")
			w.WriteHeader(http.StatusUnauthorized)
			return
		case sRes.Status.Code != rpc.Code_CODE_OK:
			log.Error().Str("token", token).Interface("status", sRes.Status).Msg("grpc stat request failed")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		log.Debug().Interface("statInfo", sRes.Info).Msg("Stat info from public link token path")

		if sRes.Info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			r = r.WithContext(context.WithValue(r.Context(), tokenStatInfoKey{}, sRes.Info))
			s.davHandler.PublicFileHandler.Handler(s).ServeHTTP(w, r)
			return
		}
		s.davHandler.PublicFolderHandler.Handler(s).ServeHTTP(w, r)
	}
}
