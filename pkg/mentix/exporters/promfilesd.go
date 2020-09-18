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

// PrometheusFileSDExporter implements the File Service Discovery for Prometheus exporter.
type PrometheusFileSDExporter struct {
	prometheusSDExporter
}

// Activate activates the exporter.
func (exporter *PrometheusFileSDExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	// Store Prometheus File SD specific settings
	exporter.outputFilename = conf.PrometheusFileSD.OutputFile

	return exporter.prometheusSDExporter.Activate(conf, log)
}

func createFileSDScrapeConfig(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig {
	labels := map[string]string{
		"site":         site.Name,
		"country":      site.CountryCode,
		"service_type": endpoint.Type.Name,
	}

	// If a metrics path was specified as a property, use that one by setting the corresponding label
	if metricsPath := meshdata.GetPropertyValue(endpoint.Properties, meshdata.PropertyMetricsPath, ""); len(metricsPath) > 0 {
		labels["__metrics_path__"] = metricsPath
	}

	return &prometheus.ScrapeConfig{
		Targets: []string{host},
		Labels:  labels,
	}
}

func init() {
	exporter := &PrometheusFileSDExporter{}
	initPrometheusSDExporter(&exporter.prometheusSDExporter, "Prometheus File SD", createFileSDScrapeConfig)
	registerExporter(config.ExporterIDPrometheusFileSD, exporter)
}
