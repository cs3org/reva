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
	"github.com/cs3org/reva/pkg/mentix/exporters/prometheus"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// PrometheusBlackboxSDExporter implements the Service Discovery for the Prometheus blackbox exporter.
type PrometheusBlackboxSDExporter struct {
	prometheusSDExporter
}

// Activate activates the exporter.
func (exporter *PrometheusBlackboxSDExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	// Store Prometheus blackbox SD specific settings
	exporter.outputFilename = conf.PrometheusBlackboxSD.OutputFile

	return exporter.prometheusSDExporter.Activate(conf, log)
}

func createBlackboxSDScrapeConfig(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig {
	// The URL of the service must be configured properly
	host = endpoint.URL
	if host == "" {
		return nil
	}

	labels := map[string]string{
		"site":         site.Name,
		"country":      site.CountryCode,
		"service_type": endpoint.Type.Name,
	}

	return &prometheus.ScrapeConfig{
		Targets: []string{host},
		Labels:  labels,
	}
}

func init() {
	exporter := &PrometheusBlackboxSDExporter{}
	initPrometheusSDExporter(&exporter.prometheusSDExporter, "Prometheus Blackbox SD", createBlackboxSDScrapeConfig)
	registerExporter(config.ExporterIDPrometheusBlackboxSD, exporter)
}
