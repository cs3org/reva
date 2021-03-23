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

package users

import (
	"fmt"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// Handler renders user data for the user id given in the url path
type Handler struct {
	gatewayAddr string
}

// Init initializes this and any contained handlers
func (h *Handler) Init(c *config.Config) error {
	h.gatewayAddr = c.GatewaySvc
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var user string
	user, r.URL.Path = router.ShiftPath(r.URL.Path)

	// FIXME use ldap to fetch user info
	u, ok := ctxuser.ContextGetUser(ctx)
	if !ok {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return
	}
	if user != u.Username {
		// FIXME allow fetching other users info? only for admins
		response.WriteOCSError(w, r, http.StatusForbidden, "user id mismatch", fmt.Errorf("%s tried to access %s user info endpoint", u.Id.OpaqueId, user))
		return
	}

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)
	switch head {
	case "":
		h.handleUsers(w, r, u)
		return
	case "groups":
		response.WriteOCSSuccess(w, r, &Groups{})
		return
	default:
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
		return
	}

}

// Quota holds quota information
type Quota struct {
	Free       int64   `json:"free" xml:"free"`
	Used       int64   `json:"used" xml:"used"`
	Total      int64   `json:"total" xml:"total"`
	Relative   float32 `json:"relative" xml:"relative"`
	Definition string  `json:"definition" xml:"definition"`
}

// Users holds users data
type Users struct {
	Quota       *Quota `json:"quota" xml:"quota"`
	Email       string `json:"email" xml:"email"`
	DisplayName string `json:"displayname" xml:"displayname"`
	// FIXME home should never be exposed ... even in oc 10
	// home
	TwoFactorAuthEnabled bool `json:"two_factor_auth_enabled" xml:"two_factor_auth_enabled"`
}

// Groups holds group data
type Groups struct {
	Groups []string `json:"groups" xml:"groups>element"`
}

func (h *Handler) handleUsers(w http.ResponseWriter, r *http.Request, u *userpb.User) {
	ctx := r.Context()
	sublog := appctx.GetLogger(r.Context())

	gc, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		sublog.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	getHomeRes, err := gc.GetHome(ctx, &provider.GetHomeRequest{})
	if err != nil {
		sublog.Error().Err(err).Msg("error calling GetHome")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if getHomeRes.Status.Code != rpc.Code_CODE_OK {
		ocdav.HandleErrorStatus(sublog, w, getHomeRes.Status)
		return
	}

	getQuotaRes, err := gc.GetQuota(ctx, &gateway.GetQuotaRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{
				Path: getHomeRes.Path,
			},
		},
	})
	if err != nil {
		sublog.Error().Err(err).Msg("error calling GetQuota")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if getQuotaRes.Status.Code != rpc.Code_CODE_OK {
		ocdav.HandleErrorStatus(sublog, w, getQuotaRes.Status)
		return
	}

	response.WriteOCSSuccess(w, r, &Users{
		// ocs can only return the home storage quota
		Quota: &Quota{
			Free: int64(getQuotaRes.TotalBytes - getQuotaRes.UsedBytes),
			Used: int64(getQuotaRes.UsedBytes),
			// TODO support negative values or flags for the quota to carry special meaning: -1 = uncalculated, -2 = unknown, -3 = unlimited
			// for now we can only report total and used
			Total:      int64(getQuotaRes.TotalBytes),
			Relative:   float32(float64(getQuotaRes.UsedBytes) / float64(getQuotaRes.TotalBytes)),
			Definition: "default",
		},
		DisplayName: u.DisplayName,
		Email:       u.Mail,
	})
}
