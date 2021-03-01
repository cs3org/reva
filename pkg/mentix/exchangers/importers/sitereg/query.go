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
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/accservice"
	"github.com/cs3org/reva/pkg/mentix/config"
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

func extractQueryInformation(params url.Values) (key.SiteIdentifier, int, string, error) {
	apiKey := params.Get("apiKey")
	if len(apiKey) == 0 {
		return "", 0, "", errors.Errorf("no API key specified")
	}

	// Try to get an account that is associated with the given API key; if none exists, return an error
	resp, err := accservice.Query("find", network.URLParams{"by": "apikey", "value": apiKey})
	if err != nil {
		return "", 0, "", errors.Wrap(err, "error while querying the accounts service")
	}
	if !resp.Success {
		return "", 0, "", errors.Errorf("unable to fetch account associated with the provided API key: %v", resp.Error)
	}

	// Extract email from account data; this is needed to calculate the site ID from the API key
	email := ""
	if value := accservice.GetResponseValue(resp, "account.email"); value != nil {
		email, _ = value.(string)
	}
	if len(email) == 0 {
		return "", 0, "", errors.Errorf("could not get the email address of the user account")
	}

	_, flags, _, err := key.SplitAPIKey(apiKey)
	if err != nil {
		return "", 0, "", errors.Errorf("sticky API key specified")
	}

	siteID, err := key.CalculateSiteID(apiKey, strings.ToLower(email))
	if err != nil {
		return "", 0, "", errors.Wrap(err, "unable to get site ID")
	}

	return siteID, flags, email, nil
}

func createErrorResponse(msg string, err error) (meshdata.Vector, int, []byte, error) {
	return nil, http.StatusBadRequest, network.CreateResponse(msg, network.ResponseParams{"error": err.Error()}), nil
}

// HandleRegisterSiteQuery registers a site.
func HandleRegisterSiteQuery(meshData *meshdata.MeshData, data []byte, params url.Values, conf *config.Configuration, _ *zerolog.Logger) (meshdata.Vector, int, []byte, error) {
	siteID, flags, email, err := extractQueryInformation(params)
	if err != nil {
		return createErrorResponse("INVALID_API_KEY", err)
	}

	msg := "SITE_REGISTERED"
	if meshData.FindSite(siteID) != nil {
		msg = "SITE_UPDATED"
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

	// If the corresponding setting is set, ignore registrations of ScienceMesh sites
	if siteType == meshdata.SiteTypeScienceMesh && conf.Importers.SiteRegistration.IgnoreScienceMeshSites {
		return meshdata.Vector{}, http.StatusOK, network.CreateResponse(msg, network.ResponseParams{"id": siteID}), nil
	}

	site, err := siteData.ToMeshDataSite(siteID, siteType, email)
	if err != nil {
		return createErrorResponse("INVALID_SITE_DATA", err)
	}

	meshDataUpdate := &meshdata.MeshData{Sites: []*meshdata.Site{site}}
	if err := meshDataUpdate.Verify(); err != nil {
		return createErrorResponse("INVALID_MESH_DATA", err)
	}
	meshDataUpdate.Status = meshdata.StatusDefault
	meshDataUpdate.InferMissingData()

	return meshdata.Vector{meshDataUpdate}, http.StatusOK, network.CreateResponse(msg, network.ResponseParams{"id": siteID}), nil
}

// HandleUnregisterSiteQuery unregisters a site.
func HandleUnregisterSiteQuery(meshData *meshdata.MeshData, _ []byte, params url.Values, _ *config.Configuration, _ *zerolog.Logger) (meshdata.Vector, int, []byte, error) {
	siteID, _, _, err := extractQueryInformation(params)
	if err != nil {
		return createErrorResponse("INVALID_API_KEY", err)
	}

	// The site ID must be provided in the call as well to enhance security further
	if params.Get("siteId") != siteID {
		return createErrorResponse("INVALID_SITE_ID", errors.Errorf("site ID mismatch"))
	}

	// Check if the site to be removed actually exists
	if meshData.FindSite(siteID) == nil {
		return createErrorResponse("INVALID_SITE_ID", errors.Errorf("site not found"))
	}

	// To remove a site, a meshdata object that contains a site with the given ID needs to be created
	site := &meshdata.Site{ID: siteID}
	meshDataUpdate := &meshdata.MeshData{Sites: []*meshdata.Site{site}}
	meshDataUpdate.Status = meshdata.StatusObsolete

	return meshdata.Vector{meshDataUpdate}, http.StatusOK, network.CreateResponse("SITE_UNREGISTERED", network.ResponseParams{"id": siteID}), nil
}
