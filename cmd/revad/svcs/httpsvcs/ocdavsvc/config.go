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
	"net/http"
)

// the config for the ocs api
func (s *svc) doConfig(w http.ResponseWriter, r *http.Request) {
	res := &ocsResponse{
		OCS: &ocsPayload{
			Meta: ocsMetaOK,
			Data: &ocsConfigData{
				// hardcoded in core as well https://github.com/owncloud/core/blob/5f0af496626b957aff38730b5771ec0a33effe31/lib/private/OCS/Config.php#L28-L34
				Version: "1.7",
				Website: "ownCloud",
				Host:    r.URL.Host, // FIXME r.URL.Host is empty
				Contact: "",
				SSL:     "false",
			},
		},
	}
	writeOCSResponse(w, r, res)
}
