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

	// UpdateMeshData is called whenever the mesh data has changed to reflect these changes.
	UpdateMeshData(*meshdata.MeshData) error
}

// BaseExporter implements basic exporter functionality common to all exporters.
type BaseExporter struct {
	exchange.BaseExchanger
}

// UpdateMeshData is called whenever the mesh data has changed to reflect these changes.
func (exporter *BaseExporter) UpdateMeshData(meshData *meshdata.MeshData) error {
	// Update the stored mesh data
	if err := exporter.storeMeshData(meshData); err != nil {
		return fmt.Errorf("unable to store the mesh data: %v", err)
	}

	return nil
}

func (exporter *BaseExporter) storeMeshData(meshData *meshdata.MeshData) error {
	// Store the new mesh data by cloning it
	meshDataCloned := meshData.Clone()
	if meshDataCloned == nil {
		return fmt.Errorf("unable to clone the mesh data")
	}
	exporter.SetMeshData(meshDataCloned)

	return nil
}
