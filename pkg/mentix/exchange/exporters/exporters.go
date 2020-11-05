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
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/entity"
	"github.com/cs3org/reva/pkg/mentix/exchange"
)

// Exporters is a vector of Exporter
type Exporters = []Exporter

var (
	registeredExporters = entity.NewRegistry()
)

// AvailableExporters returns a list of all exporters that are enabled in the configuration.
func AvailableExporters(conf *config.Configuration) ([]Exporter, error) {
	// Try to add all exporters configured in the environment
	entries, err := registeredExporters.FindEntities(conf.EnabledExporters, true, false)
	if err != nil {
		return nil, err
	}

	exporters := make([]Exporter, 0, len(entries))
	for _, entry := range entries {
		exporters = append(exporters, entry.(Exporter))
	}

	return exporters, nil
}

// ActivateExporters activates the given exporters.
func ActivateExporters(exporters []Exporter, conf *config.Configuration, log *zerolog.Logger) error {
	return exchange.ActivateExchangers(asExchangers(exporters), conf, log)
}

// StartExporters starts the given exporters.
func StartExporters(exporters []Exporter) error {
	return exchange.StartExchangers(asExchangers(exporters))
}

// StopExporters stops the given exporters.
func StopExporters(exporters []Exporter) {
	exchange.StopExchangers(asExchangers(exporters))
}

func asExchangers(exporters []Exporter) []exchange.Exchanger {
	exchangers := make([]exchange.Exchanger, 0, len(exporters))
	for _, exp := range exporters {
		exchangers = append(exchangers, exp)
	}
	return exchangers
}

// GetRequestExporters returns all exporters that implement the RequestExchanger interface.
func GetRequestExporters(exporters []Exporter) []exchange.RequestExchanger {
	return exchange.GetRequestExchangers(asExchangers(exporters))
}

func registerExporter(exporter Exporter) {
	registeredExporters.Register(exporter)
}
