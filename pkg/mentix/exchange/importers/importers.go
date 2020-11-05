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
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/entity"
	"github.com/cs3org/reva/pkg/mentix/exchange"
)

var (
	registeredImporters = entity.NewRegistry()
)

// AvailableImporters returns a list of all importers that are enabled in the configuration.
func AvailableImporters(conf *config.Configuration) ([]Importer, error) {
	// Try to add all importers configured in the environment
	entities, err := registeredImporters.FindEntities(conf.EnabledImporters, true, false)
	if err != nil {
		return nil, err
	}

	importers := make([]Importer, 0, len(entities))
	for _, entry := range entities {
		importers = append(importers, entry.(Importer))
	}

	return importers, nil
}

// ActivateImporters activates the given importers.
func ActivateImporters(importers []Importer, conf *config.Configuration, log *zerolog.Logger) error {
	return exchange.ActivateExchangers(asExchangers(importers), conf, log)
}

// StartImporters starts the given importers.
func StartImporters(importers []Importer) error {
	return exchange.StartExchangers(asExchangers(importers))
}

// StopImporters stops the given importers.
func StopImporters(importers []Importer) {
	exchange.StopExchangers(asExchangers(importers))
}

// GetRequestImporters returns all Importers that implement the RequestExchanger interface.
func GetRequestImporters(importers []Importer) []exchange.RequestExchanger {
	return exchange.GetRequestExchangers(asExchangers(importers))
}

func asExchangers(importers []Importer) []exchange.Exchanger {
	exchangers := make([]exchange.Exchanger, 0, len(importers))
	for _, imp := range importers {
		exchangers = append(exchangers, imp)
	}
	return exchangers
}

func registerImporter(importer Importer) {
	registeredImporters.Register(importer)
}
