// Copyright 2018-2023 CERN
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
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/go-chi/chi/v5"
)

// Handler renders user data for the user id given in the url path.
type Handler struct {
	gatewayAddr string
}

// Init initializes this and any contained handlers.
func (h *Handler) Init(c *config.Config) {
	h.gatewayAddr = c.GatewaySvc
}

// GetGroups handles GET requests on /cloud/users/groups
// TODO: implement.
func (h *Handler) GetGroups(w http.ResponseWriter, r *http.Request) {
	response.WriteOCSSuccess(w, r, &Groups{})
}

// Quota holds quota information.
type Quota struct {
	Free       int64   `json:"free"       xml:"free"`
	Used       int64   `json:"used"       xml:"used"`
	Total      int64   `json:"total"      xml:"total"`
	Relative   float32 `json:"relative"   xml:"relative"`
	Definition string  `json:"definition" xml:"definition"`
}

// Users holds users data.
type Users struct {
	Quota       *Quota `json:"quota"       xml:"quota"`
	Email       string `json:"email"       xml:"email"`
	DisplayName string `json:"displayname" xml:"displayname"`
	UserType    string `json:"user-type"   xml:"user-type"`
	// FIXME home should never be exposed ... even in oc 10
	// home
	TwoFactorAuthEnabled bool `json:"two_factor_auth_enabled" xml:"two_factor_auth_enabled"`
}

// Groups holds group data.
type Groups struct {
	Groups []string `json:"groups" xml:"groups>element"`
}

// GetUsers handles GET requests on /cloud/users
// Only allow self-read currently. TODO: List Users and Get on other users (both require
// administrative privileges).
func (h *Handler) GetUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sublog := appctx.GetLogger(r.Context())

	user := chi.URLParam(r, "userid")
	// FIXME use ldap to fetch user info
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return
	}
	if user != u.Username {
		// FIXME allow fetching other users info? only for admins
		response.WriteOCSError(w, r, http.StatusForbidden, "user id mismatch", fmt.Errorf("%s tried to access %s user info endpoint", u.Id.OpaqueId, user))
		return
	}

	gc, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		sublog.Error().Err(err).Msg("error getting gateway client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var total, used uint64 = 2, 1
	var relative float32
	// lightweight and federated accounts don't have access to their storage space
	if u.Id.Type != userpb.UserType_USER_TYPE_LIGHTWEIGHT && u.Id.Type != userpb.UserType_USER_TYPE_FEDERATED {
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

		getQuotaRes, err := gc.GetQuota(ctx, &gateway.GetQuotaRequest{Ref: &provider.Reference{Path: getHomeRes.Path}})
		if err != nil {
			sublog.Error().Err(err).Msg("error calling GetQuota")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if getQuotaRes.Status.Code != rpc.Code_CODE_OK {
			ocdav.HandleErrorStatus(sublog, w, getQuotaRes.Status)
			return
		}
		total = getQuotaRes.TotalBytes
		used = getQuotaRes.UsedBytes
		relative = float32(float64(used)/float64(total)) * 100
	}

	response.WriteOCSSuccess(w, r, &Users{
		// ocs can only return the home storage quota
		Quota: &Quota{
			Free: int64(total - used),
			Used: int64(used),
			// TODO support negative values or flags for the quota to carry special meaning: -1 = uncalculated, -2 = unknown, -3 = unlimited
			// for now we can only report total and used
			Total:      int64(total),
			Relative:   relative,
			Definition: "default",
		},
		DisplayName: u.DisplayName,
		Email:       u.Mail,
		UserType:    conversions.UserTypeString(u.Id.Type),
	})
}
