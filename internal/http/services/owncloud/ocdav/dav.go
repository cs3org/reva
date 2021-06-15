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
	"path"
	"strings"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"
	ctxuser "github.com/cs3org/reva/pkg/user"
	"google.golang.org/grpc/metadata"
)

type tokenStatInfoKey struct{}

// DavHandler routes to the different sub handlers
type DavHandler struct {
	AvatarsHandler      *AvatarsHandler
	FilesHandler        *WebDavHandler
	FilesHomeHandler    *WebDavHandler
	MetaHandler         *MetaHandler
	TrashbinHandler     *TrashbinHandler
	PublicFolderHandler *WebDavHandler
	PublicFileHandler   *PublicFileHandler
}

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

	h.PublicFolderHandler = new(WebDavHandler)
	if err := h.PublicFolderHandler.init("public", true); err != nil { // jail public file requests to /public/ prefix
		return err
	}

	h.PublicFileHandler = new(PublicFileHandler)
	if err := h.PublicFileHandler.init("public"); err != nil { // jail public file requests to /public/ prefix
		return err
	}

	return h.TrashbinHandler.init(c)
}

func isOwner(userIDorName string, user *userv1beta1.User) bool {
	return userIDorName != "" && (userIDorName == user.Id.OpaqueId || strings.EqualFold(userIDorName, user.Username))
}

// Handler handles requests
func (h *DavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)

		// if there is no file in the request url we assume the request url is: "/remote.php/dav/files"
		// https://github.com/owncloud/core/blob/18475dac812064b21dabcc50f25ef3ffe55691a5/tests/acceptance/features/apiWebdavOperations/propfind.feature
		if r.URL.Path == "/files" {
			log.Debug().Str("path", r.URL.Path).Msg("method not allowed")
			contextUser, ok := ctxuser.ContextGetUser(ctx)
			if ok {
				r.URL.Path = path.Join(r.URL.Path, contextUser.Username)
			}

			if r.Header.Get("Depth") == "" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				b, err := Marshal(exception{
					code:    SabredavMethodNotAllowed,
					message: "Listing members of this collection is disabled",
				})
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
			var oldPath = r.URL.Path

			// detect and check current user in URL
			requestUserID, r.URL.Path = router.ShiftPath(r.URL.Path)

			// note: some requests like OPTIONS don't forward the user
			contextUser, ok := ctxuser.ContextGetUser(ctx)
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
		case "public-files":
			base := path.Join(ctx.Value(ctxKeyBaseURI).(string), "public-files")
			ctx = context.WithValue(ctx, ctxKeyBaseURI, base)
			c, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
			}

			var res *gatewayv1beta1.AuthenticateResponse
			token, _ := router.ShiftPath(r.URL.Path)
			if _, pass, ok := r.BasicAuth(); ok {
				res, err = handleBasicAuth(r.Context(), c, token, pass)
			} else {
				q := r.URL.Query()
				sig := q.Get("signature")
				expiration := q.Get("expiration")
				// We restrict the pre-signed urls to downloads.
				if sig != "" && expiration != "" && r.Method != http.MethodGet {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				res, err = handleSignatureAuth(r.Context(), c, token, sig, expiration)
			}

			switch {
			case err != nil:
				w.WriteHeader(http.StatusInternalServerError)
				return
			case res.Status.Code == rpcv1beta1.Code_CODE_PERMISSION_DENIED:
				fallthrough
			case res.Status.Code == rpcv1beta1.Code_CODE_UNAUTHENTICATED:
				w.WriteHeader(http.StatusUnauthorized)
				return
			case res.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
				w.WriteHeader(http.StatusNotFound)
				return
			case res.Status.Code != rpcv1beta1.Code_CODE_OK:
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			ctx = tokenpkg.ContextSetToken(ctx, res.Token)
			ctx = user.ContextSetUser(ctx, res.User)
			ctx = metadata.AppendToOutgoingContext(ctx, tokenpkg.TokenHeader, res.Token)

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
