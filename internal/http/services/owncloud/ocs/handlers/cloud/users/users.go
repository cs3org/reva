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

	// TODO quite a bit to implement, but the CS3 api now has the admin api that we can use.
	var userid string
	userid, r.URL.Path = router.ShiftPath(r.URL.Path)
	switch userid {
	case "":
		switch r.Method {
		case "GET":
			// FIXME require admin
			// FIXME listusers
			h.handleGetUser(w, r, u)
		case "POST":
			// FIXME require admin
			// FIXME adduser
			response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
		default:
			response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET and POST are allowed", nil)
		}
		return
	default:
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		switch head {
		case "":
			switch r.Method {
			case "GET":
				// FIXME getuser
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "PUT":
				// FIXME require self or admin
				// FIXME edituser
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "DELETE":
				// FIXME require admin
				// FIXME deleteuser
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			default:
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, PUT and DELETE are allowed", nil)
			}
		case "enable", "disable":
			switch r.Method {
			case "PUT":
				// FIXME require admin
				// FIXME enable user, disable user
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			default:
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only PUT is allowed", nil)
			}
		case "groups":
			switch r.Method {
			case "GET":
				// FIXME require self or admin
				// FIXME list user groups
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "POST":
				// FIXME require admin
				// FIXME add to group
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "DELETE":
				// FIXME require admin
				// FIXME remove from group
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			default:
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, POST and DELETE are allowed", nil)
			}
		case "subadmins":
			// leale as stub?
			switch r.Method {
			case "GET":
				// FIXME require self or admin
				// FIXME list user groups
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "POST":
				// FIXME require admin
				// FIXME add to group
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			case "DELETE":
				// FIXME require admin
				// FIXME remove from group
				response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
			default:
				response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "Only GET, POST and DELETE are allowed", nil)
			}
		default:
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
		}
	}
}

// TODO move this to the data package and align with the user package
// Quota holds quota information
type Quota struct {
	Free       int64   `json:"free" xml:"free"`
	Used       int64   `json:"used" xml:"used"`
	Total      int64   `json:"total" xml:"total"`
	Relative   float32 `json:"relative" xml:"relative"`
	Definition string  `json:"definition" xml:"definition"`
}

// TODO move this to the data package and align with the user package
// User holds user data
type User struct {
	Enabled           string `json:"enabled" xml:"enabled"`
	UserID            string `json:"id" xml:"id"` // UserID is mapped to the preferred_name attribute in accounts
	DisplayName       string `json:"display-name" xml:"display-name"`
	LegacyDisplayName string `json:"displayname" xml:"displayname"`
	Email             string `json:"email" xml:"email"`
	Quota             *Quota `json:"quota" xml:"quota"`
	UIDNumber         int64  `json:"uidnumber" xml:"uidnumber"`
	GIDNumber         int64  `json:"gidnumber" xml:"gidnumber"`
	// FIXME home should never be exposed ... even in oc 10
	// home
	TwoFactorAuthEnabled bool `json:"two_factor_auth_enabled" xml:"two_factor_auth_enabled"`
}

// TODO move this to the data package and align with the user package
// Groups holds group data
type Groups struct {
	Groups []string `json:"groups" xml:"groups>element"`
}

func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request, u *userpb.User) {
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
	var total, used uint64
	var relative float32
	// lightweight accounts don't have access to their storage space
	if u.Id.Type != userpb.UserType_USER_TYPE_LIGHTWEIGHT {
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
		relative = float32(float64(used) / float64(total))
	}

	response.WriteOCSSuccess(w, r, &User{
		//Enabled: u.Enabled, FIXME
		UserID: u.Username,
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
		DisplayName:       u.DisplayName,
		LegacyDisplayName: u.DisplayName,
		Email:             u.Mail,
		UIDNumber:         u.UidNumber,
		GIDNumber:         u.GidNumber,
	})
}
