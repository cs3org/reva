// Copyright 2018-2024 CERN
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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"encoding/json"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

func (s *svc) getRoleDefinitions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := json.NewEncoder(w).Encode(GetBuiltinRoleDefinitionList()); err != nil {
		log := appctx.GetLogger(r.Context())
		log.Error().Err(err).Msg("error marshalling roles as json")
		handleError(ctx, err, w)
		return
	}
}
