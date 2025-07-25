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
	"net/http"
	"path"
	"path/filepath"
	"strings"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/spaces"

	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"google.golang.org/grpc/metadata"
)

type tokenStatInfoKey struct{}

// DavHandler routes to the different sub handlers.
type DavHandler struct {
	AvatarsHandler      *AvatarsHandler
	FilesHandler        *WebDavHandler
	FilesHomeHandler    *WebDavHandler
	MetaHandler         *MetaHandler
	TrashbinHandler     *TrashbinHandler
	SpacesHandler       *WebDavHandler
	PublicFolderHandler *WebDavHandler
	PublicFileHandler   *PublicFileHandler
	OCMSharesHandler    *WebDavHandler
}

const (
	ErrListingMembers     = "ERR_LISTING_MEMBERS_NOT_ALLOWED"
	ErrInvalidCredentials = "ERR_INVALID_CREDENTIALS"
	ErrMissingBasicAuth   = "ERR_MISSING_BASIC_AUTH"
	// ErrMissingBearerAuth  = "ERR_MISSING_BEARER_AUTH"
	ErrFileNotFoundInRoot = "ERR_FILE_NOT_FOUND_IN_ROOT"
)

func (h *DavHandler) init(c *Config) error {
	h.AvatarsHandler = new(AvatarsHandler)
	if err := h.AvatarsHandler.init(c); err != nil {
		return err
	}
	h.FilesHandler = new(WebDavHandler)
	if err := h.FilesHandler.init(c.FilesNamespace, false); err != nil {
		return err
	}
	h.FilesHomeHandler = new(WebDavHandler)
	if err := h.FilesHomeHandler.init(c.WebdavNamespace, true); err != nil {
		return err
	}
	h.MetaHandler = new(MetaHandler)
	if err := h.MetaHandler.init(c); err != nil {
		return err
	}
	h.TrashbinHandler = new(TrashbinHandler)

	h.SpacesHandler = new(WebDavHandler)
	if err := h.SpacesHandler.init("", false); err != nil {
		return err
	}

	h.PublicFolderHandler = new(WebDavHandler)
	if err := h.PublicFolderHandler.init("public", true); err != nil { // jail public file requests to /public/ prefix
		return err
	}

	h.PublicFileHandler = new(PublicFileHandler)
	if err := h.PublicFileHandler.init("public"); err != nil { // jail public file requests to /public/ prefix
		return err
	}

	h.OCMSharesHandler = new(WebDavHandler)
	if err := h.OCMSharesHandler.init(c.OCMNamespace, false); err != nil {
		return err
	}

	return h.TrashbinHandler.init(c)
}

func isOwner(userIDorName string, user *userv1beta1.User) bool {
	return userIDorName != "" && (userIDorName == user.Id.OpaqueId || strings.EqualFold(userIDorName, user.Username))
}

// Handler handles requests.
func (h *DavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// if there is no file in the request url we assume the request url is: "/remote.php/dav/files"
		// https://github.com/owncloud/core/blob/18475dac812064b21dabcc50f25ef3ffe55691a5/tests/acceptance/features/apiWebdavOperations/propfind.feature
		if r.URL.Path == "/files" {
			log.Debug().Str("path", r.URL.Path).Msg("method not allowed")
			contextUser, ok := appctx.ContextGetUser(ctx)
			if ok {
				r.URL.Path = path.Join(r.URL.Path, contextUser.Username)
			}

			if r.Header.Get("Depth") == "" {
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
				_, err = w.Write(b)
				if err != nil {
					log.Error().Msgf("error writing xml response: %s", b)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				return
			}
		}

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)

		switch head {
		case "avatars":
			h.AvatarsHandler.Handler(s).ServeHTTP(w, r)
		case "files":
			var requestUserID string
			oldPath := r.URL.Path

			// detect and check current user in URL
			requestUserID, r.URL.Path = router.ShiftPath(r.URL.Path)

			// note: some requests like OPTIONS don't forward the user
			contextUser, ok := appctx.ContextGetUser(ctx)
			if ok && isOwner(requestUserID, contextUser) {
				// use home storage handler when user was detected
				base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "files", requestUserID)
				ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
				r = r.WithContext(ctx)

				h.FilesHomeHandler.Handler(s).ServeHTTP(w, r)
			} else {
				r.URL.Path = oldPath
				base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "files")
				ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
				r = r.WithContext(ctx)

				h.FilesHandler.Handler(s).ServeHTTP(w, r)
			}
		case "meta":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "meta")
			ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			h.MetaHandler.Handler(s).ServeHTTP(w, r)
		case "trash-bin":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "trash-bin")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)
			r = r.WithContext(ctx)
			h.TrashbinHandler.Handler(s).ServeHTTP(w, r)
		case "spaces":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "spaces")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)

			var head string
			head, r.URL.Path = router.ShiftPath(r.URL.Path)

			switch head {
			case "trash-bin":
				r = r.WithContext(ctx)
				h.TrashbinHandler.Handler(s).ServeHTTP(w, r)
			default:
				// path is of type: space_id/relative/path/from/space
				// the space_id is the base64 encode of the path where
				// the space is located

				_, base, ok := spaces.DecodeStorageSpaceID(head)
				if !ok {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				fullPath := filepath.Join(base, r.URL.Path)
				r.URL.Path = fullPath

				ctx = context.WithValue(ctx, ctxSpaceID, head)
				ctx = context.WithValue(ctx, ctxSpaceFullPath, fullPath)
				ctx = context.WithValue(ctx, ctxSpacePath, base)
				ctx = context.WithValue(ctx, ctxSpaceRelativePath, r.URL.Path)
				r = r.WithContext(ctx)
				h.SpacesHandler.Handler(s).ServeHTTP(w, r)
			}
		case "ocm":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "ocm")
			ctx := context.WithValue(ctx, ctxKeyBaseURI, base)

			c, err := pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
			if err != nil {
				log.Error().Err(err).Msg("error getting gateway during OCM authentication")
				w.WriteHeader(http.StatusNotFound)
				return
			}

			var token, ocmshare string
			// OCM v1.1+ (OCIS et al.).
			bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if bearer != "" {
				// Bearer token is the shared secret, path is /{shareId}/path/to/resource.
				// Here we're keeping the simpler public-share model, where the internal routing is done via the token,
				// therefore we strip the shareId and reinject the token.
				// TODO(lopresti) We should instead perform a lookup via shareId and leave the token just for auth.
				var relPath string
				token = bearer
				ocmshare, relPath = router.ShiftPath(r.URL.Path)
				r.URL.Path = filepath.Join("/", token, relPath)
			} else {
				username, _, ok := r.BasicAuth()
				if ok {
					// OCM v1.0 (OC10 and Nextcloud) uses basic auth for carrying the shared secret,
					// and does not pass the shareId.
					token = username
					r.URL.Path = filepath.Join("/", token, r.URL.Path)
				} else {
					// compatibility for ScienceMesh: no auth, shared secret is the first element
					// of the path, the shareId is not given. Leave the URL as is.
					token = strings.Split(r.URL.Path, "/")[1]
				}
			}

			authRes, err := handleOCMAuth(ctx, c, ocmshare, token)
			switch {
			case err != nil:
				log.Error().Err(err).Msg("error during OCM authentication")
				w.WriteHeader(http.StatusInternalServerError)
				return
			case authRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
				log.Debug().Str("token", token).Msg("permission denied")
				fallthrough
			case authRes.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
				log.Debug().Str("token", token).Msg("unauthorized")
				w.WriteHeader(http.StatusUnauthorized)
				return
			case authRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
				log.Debug().Str("token", token).Msg("not found")
				w.WriteHeader(http.StatusNotFound)
				return
			case authRes.Status.Code != rpc.Code_CODE_OK:
				log.Error().Str("token", token).Interface("status", authRes.Status).Msg("grpc auth request failed")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx = appctx.ContextSetToken(ctx, authRes.Token)
			ctx = appctx.ContextSetUser(ctx, authRes.User)
			ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, authRes.Token)
			ctx = context.WithValue(ctx, ctxOCM, true)

			log.Debug().Str("token", token).Interface("user", authRes.User).Msg("OCM user authenticated")

			r = r.WithContext(ctx)
			h.OCMSharesHandler.Handler(s).ServeHTTP(w, r)
		case "public-files":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "public-files")
			ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
			c, err := pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
			}

			var res *gatewayv1beta1.AuthenticateResponse
			token, _ := router.ShiftPath(r.URL.Path)
			var hasValidBasicAuthHeader bool
			var pass string

			if _, pass, hasValidBasicAuthHeader = r.BasicAuth(); hasValidBasicAuthHeader {
				log.Info().Str("token", token).Msg("Handling public-files DAV request with BasicAuth")
				res, err = handleBasicAuth(r.Context(), c, token, pass)
			} else {
				q := r.URL.Query()
				sig := q.Get("signature")
				expiration := q.Get("expiration")
				// We restrict the pre-signed urls to downloads.
				if sig != "" && expiration != "" && r.Method != http.MethodGet {
					w.WriteHeader(http.StatusUnauthorized)
					log.Info().Str("token", token).Msg("Client tried to use pre-signed URL for a method other than GET, which is not allowed")
					return
				}
				log.Info().Str("token", token).Str("sig", sig).Msg("Handling public-files DAV request with handleSignatureAuth()")
				res, err = handleSignatureAuth(ctx, c, token, sig, expiration)
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
			case res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
				fallthrough
			case res.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
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

			// the public share manager knew the token, but does the referenced target still exist?
			sRes, err := getTokenStatInfo(ctx, c, token)
			switch {
			case err != nil:
				log.Error().Err(err).Msg("error sending grpc stat request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			case sRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
				fallthrough
			case sRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
				log.Debug().Str("token", token).Interface("status", res.Status).Msg("resource not found")
				w.WriteHeader(http.StatusNotFound) // log the difference
				return
			case sRes.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
				log.Debug().Str("token", token).Interface("status", res.Status).Msg("unauthorized")
				w.WriteHeader(http.StatusUnauthorized)
				return
			case sRes.Status.Code != rpc.Code_CODE_OK:
				log.Error().Str("token", token).Interface("status", res.Status).Msg("grpc stat request failed")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			log.Debug().Interface("statInfo", sRes.Info).Msg("Stat info from public link token path")

			if sRes.Info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
				ctx := context.WithValue(ctx, tokenStatInfoKey{}, sRes.Info)
				r = r.WithContext(ctx)
				h.PublicFileHandler.Handler(s).ServeHTTP(w, r)
			} else {
				h.PublicFolderHandler.Handler(s).ServeHTTP(w, r)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
			b, err := Marshal(exception{
				code:    SabredavNotFound,
				message: "File not found in root",
			}, ErrFileNotFoundInRoot)
			HandleWebdavError(log, w, b, err)
		}
	})
}

func getTokenStatInfo(ctx context.Context, client gatewayv1beta1.GatewayAPIClient, token string) (*provider.StatResponse, error) {
	return client.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: path.Join("/public", token)}})
}

func handleBasicAuth(ctx context.Context, c gatewayv1beta1.GatewayAPIClient, token, pw string) (*gatewayv1beta1.AuthenticateResponse, error) {
	authenticateRequest := gatewayv1beta1.AuthenticateRequest{
		Type:         "publicshares",
		ClientId:     token,
		ClientSecret: "password|" + pw,
	}

	return c.Authenticate(ctx, &authenticateRequest)
}

func handleSignatureAuth(ctx context.Context, c gatewayv1beta1.GatewayAPIClient, token, sig, expiration string) (*gatewayv1beta1.AuthenticateResponse, error) {
	authenticateRequest := gatewayv1beta1.AuthenticateRequest{
		Type:         "publicshares",
		ClientId:     token,
		ClientSecret: "signature|" + sig + "|" + expiration,
	}

	return c.Authenticate(ctx, &authenticateRequest)
}

func handleOCMAuth(ctx context.Context, c gatewayv1beta1.GatewayAPIClient, ocmshare, token string) (*gatewayv1beta1.AuthenticateResponse, error) {
	return c.Authenticate(ctx, &gatewayv1beta1.AuthenticateRequest{
		Type:         "ocmshares",
		ClientId:     ocmshare,
		ClientSecret: token,
	})
}
