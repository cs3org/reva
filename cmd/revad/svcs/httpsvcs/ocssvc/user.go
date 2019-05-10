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

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
)

func (s *svc) doUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	u, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	res := &Response{
		OCS: &Payload{
			Meta: MetaOK,
			Data: &UserData{
				ID:          u.Username,
				DisplayName: u.DisplayName,
				Email:       u.Mail,
			},
		},
	}

	err := WriteOCSResponse(w, r, res)
	if err != nil {
		appctx.GetLogger(r.Context()).Error().Err(err).Msg("error writing ocs response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

type contextUserRequiredErr string

func (err contextUserRequiredErr) Error() string   { return string(err) }
func (err contextUserRequiredErr) IsUserRequired() {}
