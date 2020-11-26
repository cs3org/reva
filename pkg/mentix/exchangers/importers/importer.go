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
	"fmt"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/connectors"
	"github.com/cs3org/reva/pkg/mentix/exchangers"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Importer is the interface that all importers must implement.
type Importer interface {
	exchangers.Exchanger

	// MeshData returns the vector of imported mesh data.
	MeshData() meshdata.Vector

	// Process is called periodically to perform the actual import; if data has been imported, true is returned.
	Process(*connectors.Collection) (bool, error)
}

// BaseImporter implements basic importer functionality common to all importers.
type BaseImporter struct {
	exchangers.BaseExchanger

	meshData meshdata.Vector
}

// Process is called periodically to perform the actual import; if data has been imported, true is returned.
func (importer *BaseImporter) Process(connectors *connectors.Collection) (bool, error) {
	if importer.meshData == nil { // No data present for updating, so nothing to process
		return false, nil
	}

	var processErrs []string

	// Data is read, so lock it for writing during the loop
	importer.Locker().RLock()
	for _, connector := range connectors.Connectors {
		if !importer.IsConnectorEnabled(connector.GetID()) {
			continue
		}

		if err := importer.processMeshData(connector); err != nil {
			processErrs = append(processErrs, fmt.Sprintf("unable to process imported mesh data for connector '%v': %v", connector.GetName(), err))
		}
	}
	importer.Locker().RUnlock()

	importer.SetMeshData(nil)

	var err error
	if len(processErrs) != 0 {
		err = fmt.Errorf(strings.Join(processErrs, "; "))
	}
	return true, err
}

func (importer *BaseImporter) processMeshData(connector connectors.Connector) error {
	for _, meshData := range importer.meshData {
		if err := connector.UpdateMeshData(meshData); err != nil {
			return fmt.Errorf("error while updating mesh data: %v", err)
		}
	}

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
