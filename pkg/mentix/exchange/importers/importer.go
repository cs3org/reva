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

package importers

import (
	"github.com/cs3org/reva/pkg/mentix/connectors"
	"github.com/cs3org/reva/pkg/mentix/exchange"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Importer is the interface that all importers must implement.
type Importer interface {
	exchange.Exchanger

	// MeshData returns the vector of imported mesh data.
	MeshData() meshdata.Vector

	// Process is called periodically to perform the actual import.
	Process([]connectors.Connector) error
}

// BaseImporter implements basic importer functionality common to all importers.
type BaseImporter struct {
	exchange.BaseExchanger

	meshData meshdata.Vector
}

// Process is called periodically to perform the actual import.
func (importer *BaseImporter) Process(connectors []connectors.Connector) error {
	if meshData := importer.MeshData(); meshData != nil {
		// Data is read, so lock it for writing
		importer.Locker().RLock()

		for _, connector := range connectors {
			if !importer.IsConnectorEnabled(connector.GetID()) {
				continue
			}

			// TODO: Use Connector to add/remove site/service
		}

		importer.Locker().RUnlock()
	}

	importer.SetMeshData(nil)
	return nil
}

// MeshData returns the vector of imported mesh data.
func (importer *BaseImporter) MeshData() meshdata.Vector {
	return importer.meshData
}

// SetMeshData sets the new mesh data vector.
func (importer *BaseImporter) SetMeshData(meshData meshdata.Vector) {
	importer.Locker().Lock()
	defer importer.Locker().Unlock()

	importer.meshData = meshData
}
