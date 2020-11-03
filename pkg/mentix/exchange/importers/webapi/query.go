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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cs3org/reva/pkg/mentix/meshdata"
	"github.com/cs3org/reva/pkg/mentix/network"
)

func decodeQueryData(data []byte) (*meshdata.MeshData, error) {
	site := &meshdata.Site{}
	if err := json.Unmarshal(data, site); err != nil {
		return nil, err
	}

	// Verify that the absolute minimum of information is provided
	if site.Name == "" {
		return nil, fmt.Errorf("site name missing")
	}
	if site.Domain == "" && site.Homepage == "" {
		return nil, fmt.Errorf("site URL missing")
	}

	// Infer missing data
	if site.Homepage == "" {
		site.Homepage = fmt.Sprintf("http://www.%v", site.Domain)
	} else if site.Domain == "" {
		if URL, err := url.Parse(site.Homepage); err == nil {
			site.Domain = network.ExtractDomainFromURL(URL, false)
		}
	}

	return &meshdata.MeshData{Sites: []*meshdata.Site{site}}, nil
}

func handleQuery(data []byte, params url.Values, flags int32, msg string) (meshdata.Vector, int, []byte, error) {
	meshData, err := decodeQueryData(data)
	if err != nil {
		return nil, http.StatusBadRequest, network.CreateResponse("INVALID_DATA", network.ResponseParams{"error": err.Error()}), nil
	}
	meshData.Flags = flags
	return meshdata.Vector{meshData}, http.StatusOK, network.CreateResponse(msg, network.ResponseParams{"id": meshData.Sites[0].GetID()}), nil
}

// HandleRegisterSiteQuery registers a site.
func HandleRegisterSiteQuery(data []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	return handleQuery(data, params, meshdata.FlagsNone, "SITE_REGISTERED")
}

// HandleUnregisterSiteQuery unregisters a site.
func HandleUnregisterSiteQuery(data []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	return handleQuery(data, params, meshdata.FlagObsolete, "SITE_UNREGISTERED")
}
