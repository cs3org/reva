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

	"github.com/cs3org/reva/pkg/mentix/exchange"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Exporter is the interface that all exporters must implement.
type Exporter interface {
	exchange.Exchanger

	// Update is called whenever the mesh data set has changed to reflect these changes.
	Update(meshdata.MeshDataSet) error
}

// BaseExporter implements basic exporter functionality common to all exporters.
type BaseExporter struct {
	exchange.BaseExchanger
}

// Update is called whenever the mesh data set has changed to reflect these changes.
func (exporter *BaseExporter) Update(meshDataSet meshdata.MeshDataSet) error {
	// Update the stored mesh data set
	if err := exporter.storeMeshDataSet(meshDataSet); err != nil {
		return fmt.Errorf("unable to store the mesh data: %v", err)
	}

	return nil
}

func (exporter *BaseExporter) storeMeshDataSet(meshDataSet meshdata.MeshDataSet) error {
	// Store the new mesh data set by cloning it and then merging the cloned data into one object
	meshDataSetCloned := make(meshdata.MeshDataSet)
	for connectorID, meshData := range meshDataSet {
		if !exporter.IsConnectorEnabled(connectorID) {
			continue
		}

		meshDataCloned := meshData.Clone()
		if meshDataCloned == nil {
			return fmt.Errorf("unable to clone the mesh data")
		}
		meshDataSetCloned[connectorID] = meshDataCloned
	}
	exporter.SetMeshData(meshdata.MergeMeshDataSet(meshDataSetCloned))

	return nil
}
