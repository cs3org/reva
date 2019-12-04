// Copyright 2018-2019 CERN
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

package ocs

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/user"
)

// The UserHandler renders the user endpoint
type UserHandler struct {
}

func (h *UserHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO move user to handler parameter?
	u, ok := user.ContextGetUser(ctx)
	if !ok {
		WriteOCSError(w, r, MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return
	}

	WriteOCSSuccess(w, r, &UserData{
		ID:          u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Mail,
	})
}

// UserData holds user data
type UserData struct {
	// TODO needs better naming, clarify if we need a userid, a username or both
	ID          string `json:"id" xml:"id"`
	DisplayName string `json:"display-name" xml:"display-name"`
	Email       string `json:"email" xml:"email"`
}
