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

package connectors

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

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

	return nil
}

// RetrieveMeshData fetches new mesh data.
func (connector *LocalFileConnector) RetrieveMeshData() (*meshdata.MeshData, error) {
	meshData := new(meshdata.MeshData)

	jsonFile, err := os.Open(connector.filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to open input file '%v': %v", connector.filePath, err)
	}
	defer jsonFile.Close()

	jsonData, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read input file '%v': %v", connector.filePath, err)
	}

	if err := json.Unmarshal(jsonData, &meshData.Sites); err != nil {
		return nil, fmt.Errorf("invalid input file '%v': %v", connector.filePath, err)
	}

	// Update the site types, as these are not part of the JSON data
	connector.setSiteTypes(meshData)

	return meshData, nil
}

func (connector *LocalFileConnector) setSiteTypes(meshData *meshdata.MeshData) {
	for _, site := range meshData.Sites {
		site.Type = meshdata.SiteTypeCommunity // Sites coming from a local file are always community sites
	}
}

// GetName returns the display name of the connector.
func (connector *LocalFileConnector) GetName() string {
	return "Local file"
}

func init() {
	connector := &LocalFileConnector{}
	connector.SetID(config.ConnectorIDLocalFile)
	registerConnector(connector)
}
