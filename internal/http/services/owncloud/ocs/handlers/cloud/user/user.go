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

package user

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/user"
)

// The Handler renders the user endpoint
type Handler struct {
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO move user to handler parameter?
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return
	}

	var head string
	head, r.URL.Path = router.ShiftPath(r.URL.Path)
	switch head {
	case "":
		response.WriteOCSSuccess(w, r, &User{
			// FIXME Enabled?
			UserID:            u.Username,
			DisplayName:       u.DisplayName,
			LegacyDisplayName: u.DisplayName,
			Email:             u.Mail,
			UIDNumber:         u.UidNumber,
			GIDNumber:         u.GidNumber,
		})
		return
	case "signing-key":
		// FIXME where do we store the signing key?
		// is it still nededed?  the `oc:downloadURL` webdav property is also filled ... with a different signature
		response.WriteOCSError(w, r, response.MetaUnknownError.StatusCode, "Not implemented", nil)
		return
	default:
		response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
		return
	}
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
// SigningKey holds the Payload for a GetSigningKey response
type SigningKey struct {
	User       string `json:"user" xml:"user"`
	SigningKey string `json:"signing-key" xml:"signing-key"`
}
