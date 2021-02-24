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

package sitereg

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
	"github.com/cs3org/reva/pkg/mentix/utils/network"
)

func decodeQueryData(data []byte) (*siteRegistrationData, error) {
	siteData := &siteRegistrationData{}
	if err := json.Unmarshal(data, siteData); err != nil {
		return nil, err
	}

	if err := siteData.Verify(); err != nil {
		return nil, errors.Wrap(err, "verifying the imported site data failed")
	}

	return siteData, nil
}

func decodeAPIKey(params url.Values) (key.SiteIdentifier, int8, error) {
	apiKey := params.Get("apiKey")
	if len(apiKey) == 0 {
		return "", 0, errors.Errorf("no API key specified")
	}

	// Try to get an account that is associated with the given API key; if none exists, return an error
	resp, err := queryAccountsService("find", network.URLParams{"by": "apikey", "value": apiKey})
	if err != nil {
		return "", 0, errors.Wrap(err, "error while querying the accounts service")
	}
	if !resp.Success {
		return "", 0, errors.Errorf("unable to fetch account associated with the provided API key: %v", resp.Error)
	}

	// Extract email from account data; this is needed to calculate the site ID from the API key
	email := ""
	if value := getResponseValue(resp, "account.email"); value != nil {
		email, _ = value.(string)
	}
	if len(email) == 0 {
		return "", 0, errors.Errorf("could not get the email address of the user account")
	}

	_, flags, _, err := key.SplitAPIKey(apiKey)
	if err != nil {
		return "", 0, errors.Errorf("sticky API key specified")
	}

	siteID, err := key.CalculateSiteID(apiKey, strings.ToLower(email))
	if err != nil {
		return "", 0, errors.Wrap(err, "unable to get site ID")
	}

	return siteID, flags, nil
}

func createErrorResponse(msg string, err error) (meshdata.Vector, int, []byte, error) {
	return nil, http.StatusBadRequest, network.CreateResponse(msg, network.ResponseParams{"error": err.Error()}), nil
}

// HandleRegisterSiteQuery registers a site.
func HandleRegisterSiteQuery(_ *meshdata.MeshData, data []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	siteID, flags, err := decodeAPIKey(params)
	if err != nil {
		return createErrorResponse("INVALID_API_KEY", err)
	}

	// Decode the site registration data and convert it to a meshdata object
	siteData, err := decodeQueryData(data)
	if err != nil {
		return createErrorResponse("INVALID_SITE_DATA", err)
	}

	siteType := meshdata.SiteTypeCommunity
	if flags&key.FlagScienceMesh == key.FlagScienceMesh {
		siteType = meshdata.SiteTypeScienceMesh
	}

	site, err := siteData.ToMeshDataSite(siteID, siteType)
	if err != nil {
		return createErrorResponse("INVALID_SITE_DATA", err)
	}

	meshData := &meshdata.MeshData{Sites: []*meshdata.Site{site}}
	if err := meshData.Verify(); err != nil {
		return createErrorResponse("INVALID_MESH_DATA", err)
	}
	meshData.Status = meshdata.StatusDefault
	meshData.InferMissingData()

	return meshdata.Vector{meshData}, http.StatusOK, network.CreateResponse("SITE_REGISTERED", network.ResponseParams{"id": siteID}), nil
}

// HandleUnregisterSiteQuery unregisters a site.
func HandleUnregisterSiteQuery(_ *meshdata.MeshData, _ []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	siteID, _, err := decodeAPIKey(params)
	if err != nil {
		return createErrorResponse("INVALID_API_KEY", err)
	}

	// TODO: Check if site with ID exists; bail out if not

	// The site ID must be provided in the call as well to enhance security further
	if params.Get("siteId") != siteID {
		return createErrorResponse("INVALID_SITE_ID", errors.Errorf("site ID mismatch"))
	}

	// To remove a site, a meshdata object that contains a site with the given ID needs to be created
	site := &meshdata.Site{ID: siteID}
	meshData := &meshdata.MeshData{Sites: []*meshdata.Site{site}}
	meshData.Status = meshdata.StatusObsolete

	return meshdata.Vector{meshData}, http.StatusOK, network.CreateResponse("SITE_UNREGISTERED", network.ResponseParams{"id": siteID}), nil
}
