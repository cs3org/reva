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
	"encoding/base64"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
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

const (
	ctxKeyIncomingURL = "ctxKeyIncomingURL"
)

const publicLinkSignatureParam = "oc-signature"

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
	if err := h.FilesHomeHandler.init("", false); err != nil {
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

func authenticatePublicFilesRequest(ctx context.Context, r *http.Request, c gatewayv1beta1.GatewayAPIClient, token string) (*gatewayv1beta1.AuthenticateResponse, bool, bool, error) {
	log := appctx.GetLogger(ctx)
	q := r.URL.Query()
	sig := q.Get(publicLinkSignatureParam)
	expiration := q.Get("expiration")
	_, pass, hasValidBasicAuthHeader := r.BasicAuth()

	if sig != "" || expiration != "" || !hasValidBasicAuthHeader {
		// We restrict the pre-signed urls to downloads.
		if sig != "" && expiration != "" && r.Method != http.MethodGet {
			log.Info().Str("token", token).Msg("Client tried to use pre-signed URL for a method other than GET, which is not allowed")
			return nil, hasValidBasicAuthHeader, true, nil
		}
		log.Info().Str("token", token).Str("sig", sig).Msg("Handling public-files DAV request with handleSignatureAuth()")
		res, err := handleSignatureAuth(ctx, c, token, sig, expiration)
		return res, hasValidBasicAuthHeader, false, err
	}

	log.Info().Str("token", token).Msg("Handling public-files DAV request with BasicAuth")
	res, err := handleBasicAuth(ctx, c, token, pass)
	return res, hasValidBasicAuthHeader, false, err
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

func handleOCMAuth(ctx context.Context, c gatewayv1beta1.GatewayAPIClient, ocmshare, token, authType string) (*gatewayv1beta1.AuthenticateResponse, error) {
	return c.Authenticate(ctx, &gatewayv1beta1.AuthenticateRequest{
		Type:         authType,
		ClientId:     ocmshare,
		ClientSecret: token,
	})
}

// Three non-empty base64url segments (RFC 7515 JWS Compact Serialization).
// Classifies the bearer as an exchanged JWT vs a legacy direct secret.
// Heuristic only; the exchanged-token auth manager still verifies the JWT.
func isJWT(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 {
			return false
		}
		if _, err := base64.RawURLEncoding.DecodeString(p); err != nil {
			return false
		}
	}
	return true
}

// Legacy direct-secret DAV access still routes internally by token. Exchanged-
// token access routes by share ID because code-flow tokens do not carry the
// shared secret.
func ocmInternalPath(authType, token, shareID, relPath string) (string, bool) {
	internalPath := filepath.Join("/", token, relPath)
	if authType == "ocmexchangedtoken" {
		return filepath.Join("/", shareID, relPath), true
	}
	return internalPath, false
}
