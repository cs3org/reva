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

package cs3api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// HandleDefaultQuery processes a basic query.
func HandleDefaultQuery(meshData *meshdata.MeshData, params url.Values, _ *config.Configuration, _ *zerolog.Logger) (int, []byte, error) {
	// Convert the mesh data
	ocmData, err := convertMeshDataToOCMData(meshData)
	if err != nil {
		return http.StatusBadRequest, []byte{}, fmt.Errorf("unable to convert the mesh data to OCM data structures: %v", err)
	}

	// Marshal the OCM data as JSON
	data, err := json.MarshalIndent(ocmData, "", "\t")
	if err != nil {
		return http.StatusBadRequest, []byte{}, fmt.Errorf("unable to marshal the OCM data: %v", err)
	}

	return http.StatusOK, data, nil
}

func convertMeshDataToOCMData(meshData *meshdata.MeshData) ([]*ocmprovider.ProviderInfo, error) {
	// Convert the mesh data into the corresponding OCM data structures
	providers := make([]*ocmprovider.ProviderInfo, 0, len(meshData.Sites))
	for _, site := range meshData.Sites {
		// Gather all services from the site
		services := make([]*ocmprovider.Service, 0, len(site.Services))
		for _, service := range site.Services {
			// Gather all additional endpoints of the service
			addEndpoints := make([]*ocmprovider.ServiceEndpoint, 0, len(service.AdditionalEndpoints))
			for _, endpoint := range service.AdditionalEndpoints {
				addEndpoints = append(addEndpoints, convertServiceEndpointToOCMData(endpoint))
			}

			services = append(services, &ocmprovider.Service{
				Host:                service.Host,
				Endpoint:            convertServiceEndpointToOCMData(service.ServiceEndpoint),
				AdditionalEndpoints: addEndpoints,
				ApiVersion:          meshdata.GetPropertyValue(service.Properties, meshdata.PropertyAPIVersion, ""),
			})
		}

		// Copy the site info into a ProviderInfo
		providers = append(providers, &ocmprovider.ProviderInfo{
			Name:         site.Name,
			FullName:     site.FullName,
			Description:  site.Description,
			Organization: site.Organization,
			Domain:       site.Domain,
			Homepage:     site.Homepage,
			Email:        site.Email,
			Services:     services,
			Properties:   site.Properties,
		})
	}

	return providers, nil
}

func convertServiceEndpointToOCMData(endpoint *meshdata.ServiceEndpoint) *ocmprovider.ServiceEndpoint {
	return &ocmprovider.ServiceEndpoint{
		Type: &ocmprovider.ServiceType{
			Name:        endpoint.Type.Name,
			Description: endpoint.Type.Description,
		},
		Name:        endpoint.Name,
		Path:        endpoint.URL,
		IsMonitored: endpoint.IsMonitored,
		Properties:  endpoint.Properties,
	}
}
