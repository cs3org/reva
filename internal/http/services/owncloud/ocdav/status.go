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

package ocdav

import (
	"encoding/json"
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/data"
	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) doStatus(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	status := &data.Status{
		Installed:      true,
		Maintenance:    false,
		NeedsDBUpgrade: false,
		Version:        "10.0.9.5", // TODO(jfd) make build/config determined
		VersionString:  "10.0.9",
		Edition:        "community",
		ProductName:    "ownCloud",
	}

	statusJSON, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(statusJSON); err != nil {
		log.Err(err).Msg("error writing response")
	}
}
