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
	"net/http"
	"strings"
)

func (s *svc) handleOptions(w http.ResponseWriter, r *http.Request, ns string) {
	allow := "OPTIONS, LOCK, GET, HEAD, POST, DELETE, PROPPATCH, COPY,"
	allow += " MOVE, UNLOCK, PROPFIND, MKCOL, REPORT, SEARCH,"
	allow += " PUT" // TODO(jfd): only for files ... but we cannot create the full path without a user ... which we only have when credentials are sent

	isPublic := strings.Contains(r.Context().Value(ctxKeyBaseURI).(string), "public-files")

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Allow", allow)
	w.Header().Set("DAV", "1, 2")
	w.Header().Set("MS-Author-Via", "DAV")
	if !isPublic {
		w.Header().Add("Access-Control-Allow-Headers", "Tus-Resumable")
		w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
		w.Header().Set("Tus-Resumable", "1.0.0") // TODO(jfd): only for dirs?
		w.Header().Set("Tus-Version", "1.0.0")
		w.Header().Set("Tus-Extension", "creation,creation-with-upload")
	}
	w.WriteHeader(http.StatusOK)
}
