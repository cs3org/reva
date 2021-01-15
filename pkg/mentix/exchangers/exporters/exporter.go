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

package exporters

import (
	"fmt"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/exchangers"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Exporter is the interface that all exporters must implement.
type Exporter interface {
	exchangers.Exchanger

	// MeshData returns the mesh data.
	MeshData() *meshdata.MeshData

	// Update is called whenever the mesh data set has changed to reflect these changes.
	Update(meshdata.Map) error
}

// BaseExporter implements basic exporter functionality common to all exporters.
type BaseExporter struct {
	exchangers.BaseExchanger

	meshData *meshdata.MeshData

	allowUnauthorizedSites bool
}

// Update is called whenever the mesh data set has changed to reflect these changes.
func (exporter *BaseExporter) Update(meshDataSet meshdata.Map) error {
	// Update the stored mesh data set
	if err := exporter.storeMeshDataSet(meshDataSet); err != nil {
		return fmt.Errorf("unable to store the mesh data: %v", err)
	}

	return nil
}

func (exporter *BaseExporter) storeMeshDataSet(meshDataSet meshdata.Map) error {
	// Store the new mesh data set by cloning it and then merging the cloned data into one object
	meshDataSetCloned := make(meshdata.Map)
	for connectorID, meshData := range meshDataSet {
		if !exporter.IsConnectorEnabled(connectorID) {
			continue
		}

		meshDataCloned := meshData.Clone()
		if meshDataCloned == nil {
			return fmt.Errorf("unable to clone the mesh data")
		}

		if !exporter.allowUnauthorizedSites {
			exporter.removeUnauthorizedSites(meshDataCloned)
		}

		meshDataSetCloned[connectorID] = meshDataCloned
	}
	exporter.SetMeshData(meshdata.MergeMeshDataMap(meshDataSetCloned))

	return nil
}

// MeshData returns the stored mesh data.
func (exporter *BaseExporter) MeshData() *meshdata.MeshData {
	return exporter.meshData
}

// SetMeshData sets new mesh data.
func (exporter *BaseExporter) SetMeshData(meshData *meshdata.MeshData) {
	exporter.Locker().Lock()
	defer exporter.Locker().Unlock()

	exporter.meshData = meshData
}

func (exporter *BaseExporter) removeUnauthorizedSites(meshData *meshdata.MeshData) {
	cleanedSites := make([]*meshdata.Site, 0, len(meshData.Sites))
	for _, site := range meshData.Sites {
		// Only keep authorized sites
		if value := meshdata.GetPropertyValue(site.Properties, meshdata.PropertyAuthorized, "false"); strings.EqualFold(value, "true") {
			cleanedSites = append(cleanedSites, site)
		}
	}
	meshData.Sites = cleanedSites
}
