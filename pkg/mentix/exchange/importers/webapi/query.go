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

package webapi

import (
	"net/http"
	"net/url"

	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// HandleRegisterSiteQuery registers a site.
func HandleRegisterSiteQuery(params url.Values) (meshdata.Vector, int, []byte, error) {
	// TODO: Handlen + Response (ähnlich OCM)
	return meshdata.Vector{&meshdata.MeshData{[]*meshdata.Site{&meshdata.Site{Name: "TEST"}}, []*meshdata.ServiceType{}, 0}}, http.StatusOK, []byte("registered"), nil
}

// HandleUnregisterSiteQuery unregisters a site.
func HandleUnregisterSiteQuery(params url.Values) (meshdata.Vector, int, []byte, error) {
	// TODO: Handlen + Response (ähnlich OCM)
	return meshdata.Vector{}, http.StatusOK, []byte("unregistered"), nil
}
