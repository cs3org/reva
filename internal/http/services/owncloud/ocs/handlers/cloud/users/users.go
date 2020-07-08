// Copyright 2018-2020 CERN
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

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rhttp/router"
	ctxuser "github.com/cs3org/reva/pkg/user"
)

// The UsersHandler renders user data for the user id given in the url path
type Handler struct {
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
		// FIXME allow fetching other users info?
		response.WriteOCSError(w, r, http.StatusForbidden, "user id mismatch", fmt.Errorf("%s tried to access %s user info endpoint", u.Id.OpaqueId, user))
		return
	}

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)
	switch head {
	case "":
		response.WriteOCSSuccess(w, r, &Users{
			// FIXME query storages? cache a summary?
			// TODO use list of storages to allow clients to resolve quota status
			Quota: &Quota{
				Free:       2840756224000,
				Used:       5059416668,
				Total:      2845815640668,
				Relative:   0.18,
				Definition: "default",
			},
			DisplayName: u.DisplayName,
			Email:       u.Mail,
		})
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
	//home
	TwoFactorAuthEnabled bool `json:"two_factor_auth_enabled" xml:"two_factor_auth_enabled"`
}

// Groups holds group data
type Groups struct {
	Groups []string `json:"groups" xml:"groups>element"`
}
