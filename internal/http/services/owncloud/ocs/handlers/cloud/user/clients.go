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
	"net/http"
	"strings"
	"time"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/go-chi/chi/v5"
)

// Client is one connected sync client, backed by an app password. The three
// fields are decoded from the app password label "<name>|<description>|<id>":
//   - Name is the device name the user chose when connecting (may be empty).
//   - Description is the human-readable client parsed from its User-Agent.
//   - ID is the client_id, used as the row id for revocation.
type Client struct {
	ID          string `json:"id"           xml:"id"`
	Name        string `json:"name"         xml:"name"`
	Description string `json:"description"  xml:"description"`
	CreatedAt   string `json:"created_at"   xml:"created_at"`
	LastSeenAt  string `json:"last_seen_at" xml:"last_seen_at"`
}

// ListClients handles GET /cloud/user/clients.
func (h *Handler) ListClients(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting gateway client", err)
		return
	}

	res, err := gw.ListAppPasswords(ctx, &appauthpb.ListAppPasswordsRequest{})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error listing app passwords", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, res.Status.Message, nil)
		return
	}

	clients := make([]Client, 0, len(res.AppPasswords))
	for _, pw := range res.AppPasswords {
		name, description, cid := parseLabel(pw.Label)
		clients = append(clients, Client{
			ID:          cid,
			Name:        name,
			Description: description,
			CreatedAt:   unixToRFC3339(pw.GetCtime().GetSeconds()),
			LastSeenAt:  unixToRFC3339(pw.GetUtime().GetSeconds()),
		})
	}

	response.WriteOCSSuccess(w, r, clients)
}

// DeleteClient handles DELETE /cloud/user/clients/{cid}. It is idempotent: a
// missing client returns 204, not 500.
func (h *Handler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cid := chi.URLParam(r, "cid")

	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error getting gateway client", err)
		return
	}

	res, err := gw.ListAppPasswords(ctx, &appauthpb.ListAppPasswordsRequest{})
	if err != nil {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "error listing app passwords", err)
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, res.Status.Message, nil)
		return
	}

	var secret string
	for _, pw := range res.AppPasswords {
		if _, _, c := parseLabel(pw.Label); c == cid {
			secret = pw.Password
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

	appctx.GetLogger(ctx).Info().Str("client_id", cid).Msg("client revoked")
	w.WriteHeader(http.StatusNoContent)
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
