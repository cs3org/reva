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

package connectors

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// LocalFileConnector is used to read sites from a local file.
type LocalFileConnector struct {
	BaseConnector

	filePath string
}

// Activate activates the connector.
func (connector *LocalFileConnector) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := connector.BaseConnector.Activate(conf, log); err != nil {
		return err
	}

	// Check and store GOCDB specific settings
	connector.filePath = conf.Connectors.LocalFile.File
	if len(connector.filePath) == 0 {
		return fmt.Errorf("no file configured")
	}

	// Create the file directory if necessary
	dir := filepath.Dir(connector.filePath)
	_ = os.MkdirAll(dir, 0755)

	// Create an empty file if it doesn't exist
	if _, err := os.Stat(connector.filePath); os.IsNotExist(err) {
		_ = ioutil.WriteFile(connector.filePath, []byte("[]"), 0755)
	}

	return nil
}

// RetrieveMeshData fetches new mesh data.
func (connector *LocalFileConnector) RetrieveMeshData() (*meshdata.MeshData, error) {
	meshData := &meshdata.MeshData{}

	jsonData, err := ioutil.ReadFile(connector.filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file '%v': %v", connector.filePath, err)
	}

	if err := json.Unmarshal(jsonData, &meshData.Sites); err != nil {
		return nil, fmt.Errorf("invalid file '%v': %v", connector.filePath, err)
	}

	// Update the site types, as these are not part of the JSON data
	connector.setSiteTypes(meshData)

	meshData.InferMissingData()
	return meshData, nil
}

// UpdateMeshData updates the provided mesh data on the target side.
func (connector *LocalFileConnector) UpdateMeshData(updatedData *meshdata.MeshData) error {
	meshData, err := connector.RetrieveMeshData()
	if err != nil {
		// Ignore errors and start with an empty data set
		meshData = &meshdata.MeshData{}
	}

	err = nil
	switch updatedData.Status {
	case meshdata.StatusDefault:
		err = connector.mergeData(meshData, updatedData)

	case meshdata.StatusObsolete:
		err = connector.unmergeData(meshData, updatedData)

	case meshdata.StatusAuthorize:
		err = connector.authorizeData(meshData, updatedData, true)

	case meshdata.StatusUnauthorize:
		err = connector.authorizeData(meshData, updatedData, false)
	}

	if err != nil {
		return err
	}

	// Write the updated sites back to the file
	jsonData, _ := json.MarshalIndent(meshData.Sites, "", "\t")
	if err := ioutil.WriteFile(connector.filePath, jsonData, 0755); err != nil {
		return fmt.Errorf("unable to write file '%v': %v", connector.filePath, err)
	}

	return nil
}

func (connector *LocalFileConnector) mergeData(meshData *meshdata.MeshData, updatedData *meshdata.MeshData) error {
	// Store the previous authorization status for already existing sites
	siteAuthorizationStatus := make(map[string]string)
	for _, site := range meshData.Sites {
		siteAuthorizationStatus[site.ID] = meshdata.GetPropertyValue(site.Properties, meshdata.PropertyAuthorized, "false")
	}

	// Add/update data by merging
	meshData.Merge(updatedData)

	// Restore the authorization status for all sites
	for siteID, status := range siteAuthorizationStatus {
		if site := meshData.FindSite(siteID); site != nil {
			meshdata.SetPropertyValue(&site.Properties, meshdata.PropertyAuthorized, status)
		}
	}
	return nil
}

func (connector *LocalFileConnector) unmergeData(meshData *meshdata.MeshData, updatedData *meshdata.MeshData) error {
	// Remove data by unmerging
	meshData.Unmerge(updatedData)
	return nil
}

func (connector *LocalFileConnector) authorizeData(meshData *meshdata.MeshData, updatedData *meshdata.MeshData, authorize bool) error {
	for _, placeholderSite := range updatedData.Sites {
		// The site ID is stored in the updated site's name
		if site := meshData.FindSite(placeholderSite.Name); site != nil {
			if authorize {
				meshdata.SetPropertyValue(&site.Properties, meshdata.PropertyAuthorized, "true")
			} else {
				meshdata.SetPropertyValue(&site.Properties, meshdata.PropertyAuthorized, "false")
			}
		} else {
			return fmt.Errorf("no site with id '%v' found", placeholderSite.Name)
		}
	}

	return nil
}

func (connector *LocalFileConnector) setSiteTypes(meshData *meshdata.MeshData) {
	for _, site := range meshData.Sites {
		site.Type = meshdata.SiteTypeCommunity // Sites coming from a local file are always community sites
	}
}

// GetID returns the ID of the connector.
func (connector *LocalFileConnector) GetID() string {
	return config.ConnectorIDLocalFile
}

// GetName returns the display name of the connector.
func (connector *LocalFileConnector) GetName() string {
	return "Local file"
}

func init() {
	registerConnector(&LocalFileConnector{})
}
