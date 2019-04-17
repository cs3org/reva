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

package ocdavsvc

import (
	"encoding/json"
	"net/http"
)

func (s *svc) doStatus(w http.ResponseWriter, r *http.Request) {
	status := &ocsStatus{
		Installed:      true,
		Maintenance:    false,
		NeedsDBUpgrade: false,
		Version:        "10.0.9.5",  // TODO make build determined
		VersionString:  "10.0.9",    // TODO make build determined
		Edition:        "community", // TODO make build determined
		ProductName:    "ownCloud",  // TODO make configurable
	}

	statusJSON, err := json.MarshalIndent(status, "", "    ")
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(statusJSON)
}
