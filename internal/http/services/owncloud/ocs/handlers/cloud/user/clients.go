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

package user

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appctx"
	authcache "github.com/cs3org/reva/v3/pkg/auth/cache"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// Client is one app password. For one enrolled through the login flow, three
// fields are decoded from the label "<name>|<description>|<id>":
//   - Name is the device name the user chose when connecting (may be empty).
//   - Description is the human-readable client parsed from its User-Agent.
//   - ID is the client_id, used as the row id for revocation.
//
// For any other, Description is the label and ID the password id.
type Client struct {
	ID          string   `json:"id"           xml:"id"`
	Name        string   `json:"name"         xml:"name"`
	Description string   `json:"description"  xml:"description"`
	CreatedAt   string   `json:"created_at"   xml:"created_at"`
	LastSeenAt  string   `json:"last_seen_at" xml:"last_seen_at"`
	PasswordID  string   `json:"password_id"  xml:"password_id"`
	Kind        string   `json:"client"       xml:"client"`
	Permissions []string `json:"permissions"  xml:"permissions>element"`
	Roots       []string `json:"roots"        xml:"roots>element"`
	ExpiresAt   string   `json:"expires_at"   xml:"expires_at"`
	CreatedBy   string   `json:"created_by"   xml:"created_by"`
}

// ListClients handles GET /cloud/user/clients.
func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	gw, err := service.Gateway(ctx)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting gateway client", err)
		return
	}

	res, err := gw.ListAppPasswords(ctx, &appauthpb.ListAppPasswordsRequest{})
	if err != nil {
		writeGatewayError(w, r, "error listing app passwords", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, res.Status.Message, nil)
		return
	}

	var creators map[string]string
	if e := res.GetOpaque().GetMap()[appauth.OpaqueCreatedBy]; e != nil {
		if err := json.Unmarshal(e.Value, &creators); err != nil {
			response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error decoding app password creators", err)
			return
		}
	}

	clients := make([]Client, 0, len(res.AppPasswords))
	for _, pw := range res.AppPasswords {
		c := toClient(pw, "")
		c.CreatedBy = creators[c.PasswordID]
		clients = append(clients, c)
	}

	response.WriteOCSSuccess(w, r, clients)
}

func toClient(pw *appauthpb.AppPassword, createdBy string) Client {
	name, description, cid := parseLabel(pw.Label)
	c := Client{
		ID:          cid,
		Name:        name,
		Description: description,
		CreatedAt:   unixToRFC3339(pw.GetCtime().GetSeconds()),
		LastSeenAt:  unixToRFC3339(pw.GetUtime().GetSeconds()),
		ExpiresAt:   unixToRFC3339(pw.GetExpiration().GetSeconds()),
		CreatedBy:   createdBy,
	}
	if ap, ok := scope.GetAppAuthScope(pw.TokenScope); ok {
		c.PasswordID, c.Kind, c.Permissions, c.Roots = ap.PasswordID, ap.Client, ap.Permissions, ap.Roots
	}
	if c.Permissions == nil {
		c.Permissions = []string{}
	}
	if c.Roots == nil {
		c.Roots = []string{}
	}
	if c.ID == "" {
		c.ID = c.PasswordID
	}
	return c
}

// DeleteClient handles DELETE /cloud/user/clients/{cid}, where cid is a password
// id or, for an app password enrolled through the login flow, its client id. It
// is idempotent: a missing client returns 204, not 500.
func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cid := chi.URLParam(r, "cid")

	gw, err := service.Gateway(ctx)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting gateway client", err)
		return
	}

	res, err := gw.ListAppPasswords(ctx, &appauthpb.ListAppPasswordsRequest{})
	if err != nil {
		writeGatewayError(w, r, "error listing app passwords", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, res.Status.Message, nil)
		return
	}

	var secret, passwordID string
	for _, pw := range res.AppPasswords {
		ap, _ := scope.GetAppAuthScope(pw.TokenScope)
		if _, _, c := parseLabel(pw.Label); c == cid || (ap != nil && ap.PasswordID == cid) {
			secret = pw.Password
			if ap != nil {
				passwordID = ap.PasswordID
			}
			break
		}
	}
	if secret == "" {
		// Already revoked or never existed: nothing to do.
		w.WriteHeader(http.StatusNoContent)
		return
	}

	inv, err := gw.InvalidateAppPassword(ctx, &appauthpb.InvalidateAppPasswordRequest{Password: secret})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error invalidating app password", err)
		return
	}
	if inv.Status.Code != rpc.Code_CODE_OK && inv.Status.Code != rpc.Code_CODE_NOT_FOUND {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, inv.Status.Message, nil)
		return
	}

	if passwordID != "" {
		forgetAppPassword(passwordID)
	}
	appctx.GetLogger(ctx).Info().Str("client_id", cid).Str("password_id", passwordID).Msg("client revoked")
	w.WriteHeader(http.StatusNoContent)
}

// writeGatewayError answers 403 when the gateway refused the session, as it
// refuses app password management to an app password.
func writeGatewayError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if grpcstatus.Code(err) == codes.PermissionDenied {
		response.WriteOCSError(w, r, http.StatusForbidden, msg, err)
		return
	}
	response.WriteOCSError(w, r, response.MetaServerError.StatusCode, msg, err)
}

// forgetAppPassword drops the identities this node cached for an app password,
// so that it stops working here at once.
func forgetAppPassword(passwordID string) {
	authcache.Forget(func(id *authcache.Identity) bool {
		ap, ok := scope.GetAppAuthScope(id.Scopes)
		return ok && ap.PasswordID == passwordID
	})
}

// parseLabel decodes the "<name>|<description>|<client_id>" app password label.
// name and client_id never contain "|" (the name is validated at grant, the
// client_id is a UUID), so name is taken before the first "|" and client_id
// after the last; whatever is between is the description. A label that does not
// match this shape is returned whole as the description.
func parseLabel(label string) (name, description, cid string) {
	first := strings.IndexByte(label, '|')
	last := strings.LastIndexByte(label, '|')
	if first < 0 || first == last {
		return "", label, ""
	}
	return label[:first], label[first+1 : last], label[last+1:]
}

func unixToRFC3339(sec uint64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(int64(sec), 0).UTC().Format(time.RFC3339)
}
