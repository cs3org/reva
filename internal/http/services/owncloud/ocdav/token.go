// Copyright 2018-2022 CERN
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
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/v2/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/v2/pkg/ctx"
	"github.com/cs3org/reva/v2/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/utils"
	"google.golang.org/grpc/metadata"
)

// TokenHandler handles requests for public link tokens
type TokenHandler struct{}

// Handler handles http requests
func (t TokenHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		typ, tkn := router.ShiftPath(r.URL.Path)
		tkn, _ = router.ShiftPath(tkn)

		c, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch typ {
		case "protected":
			s.handleGetToken(w, r, tkn, c, true)
		case "unprotected":
			s.handleGetToken(w, r, tkn, c, false)
		}
	})
}

// TokenInfo contains information about the token
type TokenInfo struct {
	// for all callers
	Token             string `xml:"token"`
	LinkURL           string `xml:"linkurl"`
	PasswordProtected bool   `xml:"passwordprotected"`

	// if not password protected
	StorageID string `xml:"storageid"`
	OpaqueID  string `xml:"opaqueid"`
	Path      string `xml:"path"`

	// if native access
	SpacePath  string `xml:"spacePath"`
	SpaceAlias string `xml:"spaceAlias"`
	SpaceURL   string `xml:"spaceURL"`
}

func (s *svc) handleGetToken(w http.ResponseWriter, r *http.Request, tkn string, c gateway.GatewayAPIClient, protected bool) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	user, token, passwordProtected, err := getInfoForToken(tkn, r.URL.Query(), c)
	if err != nil {
		log.Error().Err(err).Msg("error stating token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	t, err := buildTokenInfo(user, tkn, token, passwordProtected, c)
	if err != nil {
		log.Error().Err(err).Msg("error stating resource behind token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if protected && !t.PasswordProtected {
		space, status, err := spacelookup.LookUpStorageSpaceByID(ctx, c, t.StorageID)
		// add info only if user is able to stat
		if err == nil && status.Code == rpc.Code_CODE_OK {
			t.SpacePath = utils.ReadPlainFromOpaque(space.Opaque, "path")
			t.SpaceAlias = utils.ReadPlainFromOpaque(space.Opaque, "spaceAlias")
			t.SpaceURL = path.Join(t.SpaceAlias, t.OpaqueID, t.Path)
		}

	}

	b, err := xml.Marshal(t)
	if err != nil {
		log.Error().Err(err).Msg("error marshaling xml")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(b)
	w.WriteHeader(http.StatusOK)
}

func buildTokenInfo(owner *user.User, tkn string, token string, passProtected bool, c gateway.GatewayAPIClient) (TokenInfo, error) {
	t := TokenInfo{Token: tkn, LinkURL: "/s/" + tkn}
	if passProtected {
		t.PasswordProtected = true
		return t, nil
	}

	ctx := ctxpkg.ContextSetToken(context.TODO(), token)
	ctx = ctxpkg.ContextSetUser(ctx, owner)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, token)

	sRes, err := getTokenStatInfo(ctx, c, tkn)
	if err != nil || sRes.Status.Code != rpc.Code_CODE_OK {
		return t, fmt.Errorf("can't stat resource. %+v %s", sRes, err)
	}

	ls := &link.PublicShare{}
	_ = json.Unmarshal(sRes.Info.Opaque.Map["link-share"].Value, ls)

	t.StorageID = ls.ResourceId.GetStorageId()
	t.OpaqueID = ls.ResourceId.GetOpaqueId()

	return t, nil
}

func getInfoForToken(tkn string, q url.Values, c gateway.GatewayAPIClient) (owner *user.User, token string, passwordProtected bool, err error) {
	ctx := context.Background()

	sig := q.Get("signature")
	expiration := q.Get("expiration")
	res, err := handleSignatureAuth(ctx, c, tkn, sig, expiration)
	if err != nil {
		return
	}

	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		// nothing to do
	case rpc.Code_CODE_PERMISSION_DENIED:
		if res.Status.Message != "wrong password" {
			err = errors.New("not found")
			return
		}

		passwordProtected = true
		return
	default:
		err = fmt.Errorf("authentication returned unsupported status code '%d'", res.Status.Code)
		return
	}

	return res.User, res.Token, false, nil
}
