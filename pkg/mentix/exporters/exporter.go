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
	"sync"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

var (
	registeredExporters = map[string]Exporter{}
)

type Exporter interface {
	Activate(conf *config.Configuration, log *zerolog.Logger) error
	Start() error
	Stop()

	UpdateMeshData(*meshdata.MeshData) error

	GetName() string
}

type BaseExporter struct {
	conf *config.Configuration
	log  *zerolog.Logger

	meshData *meshdata.MeshData
	locker   sync.RWMutex
}

func (exporter *BaseExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	exporter.conf = conf

	if log == nil {
		return fmt.Errorf("no logger provided")
	}
	exporter.log = log

	return nil
}

func (exporter *BaseExporter) Start() error {
	return nil
}

func (exporter *BaseExporter) Stop() {

}

func (exporter *BaseExporter) UpdateMeshData(meshData *meshdata.MeshData) error {
	// Update the stored mesh data
	if err := exporter.storeMeshData(meshData); err != nil {
		return fmt.Errorf("unable to store the mesh data: %v", err)
	}

	return nil
}

func (exporter *BaseExporter) storeMeshData(meshData *meshdata.MeshData) error {
	exporter.locker.Lock()
	defer exporter.locker.Unlock()

	// Store the new mesh data by cloning it
	exporter.meshData = meshData.Clone()
	if exporter.meshData == nil {
		return fmt.Errorf("unable to clone the mesh data")
	}

	return nil
}

func registerExporter(id string, exporter Exporter) {
	registeredExporters[id] = exporter
}

func AvailableExporters(conf *config.Configuration) ([]Exporter, error) {
	// Try to add all exporters configured in the environment
	var exporters []Exporter
	for _, exporterID := range conf.Exporters {
		if exporter, ok := registeredExporters[exporterID]; ok {
			exporters = append(exporters, exporter)
		} else {
			return nil, fmt.Errorf("no exporter with ID '%v' registered", exporterID)
		}
	}

	// At least one exporter must be configured
	if len(exporters) == 0 {
		return nil, fmt.Errorf("no exporters available")
	}

	return exporters, nil
}

func RegisteredExporterIDs() []string {
	var keys []string
	for k := range registeredExporters {
		keys = append(keys, k)
	}
	return keys
}
