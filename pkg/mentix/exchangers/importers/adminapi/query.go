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

package adminapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/meshdata"
	"github.com/cs3org/reva/pkg/mentix/network"
)

func decodeAdminQueryData(data []byte) (*meshdata.MeshData, error) {
	jsonData := make(map[string]interface{})
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}

	if value, ok := jsonData["id"]; ok {
		if id, ok := value.(string); ok {
			site := &meshdata.Site{}
			site.ID = id // We only need to store the ID of the site

			meshData := &meshdata.MeshData{Sites: []*meshdata.Site{site}}
			return meshData, nil
		}

		return nil, fmt.Errorf("site id invalid")
	}

	return nil, fmt.Errorf("site id missing")
}

func handleAdminQuery(data []byte, params url.Values, status int, msg string) (meshdata.Vector, int, []byte, error) {
	meshData, err := decodeAdminQueryData(data)
	if err != nil {
		return nil, http.StatusBadRequest, network.CreateResponse("INVALID_DATA", network.ResponseParams{"error": err.Error()}), nil
	}
	meshData.Status = status
	return meshdata.Vector{meshData}, http.StatusOK, network.CreateResponse(msg, network.ResponseParams{"id": meshData.Sites[0].Name}), nil
}

// HandleAuthorizeSiteQuery sets the authorization status of a site.
func HandleAuthorizeSiteQuery(data []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	status := params.Get("status")

	if strings.EqualFold(status, "true") {
		return handleAdminQuery(data, params, meshdata.StatusAuthorize, "SITE_AUTHORIZED")
	} else if strings.EqualFold(status, "false") {
		return handleAdminQuery(data, params, meshdata.StatusUnauthorize, "SITE_UNAUTHORIZED")
	}

	return nil, http.StatusBadRequest, network.CreateResponse("INVALID_QUERY", network.ResponseParams{}), nil
}
