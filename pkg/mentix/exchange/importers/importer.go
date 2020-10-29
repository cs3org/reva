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
	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exchange"
	"github.com/cs3org/reva/pkg/mentix/util/registry"
)

var (
	registeredImporters = registry.NewRegistry()
)

// Importer is the interface that all importers must implement.
type Importer interface {
	exchange.Exchanger
}

// BaseImporter implements basic importer functionality common to all importers.
type BaseImporter struct {
	exchange.BaseExchanger
}

// AvailableImporters returns a list of all importers that are enabled in the configuration.
func AvailableImporters(conf *config.Configuration) ([]Importer, error) {
	// Try to add all importers configured in the environment
	entries, err := registeredImporters.EntriesByID(conf.EnabledImporters)
	if err != nil {
		return nil, err
	}

	importers := make([]Importer, 0, len(entries))
	for _, entry := range entries {
		importers = append(importers, entry.(Importer))
	}

	return importers, nil
}

func registerImporter(id string, exporter Importer) {
	registeredImporters.Register(id, exporter)
}
