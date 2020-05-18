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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exporters/prometheus"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

type PrometheusFileSDExporter struct {
	BaseExporter

	outputFilename string
}

func (exporter *PrometheusFileSDExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := exporter.BaseExporter.Activate(conf, log); err != nil {
		return err
	}

	// Check and store Prometheus File SD specific settings
	exporter.outputFilename = conf.PrometheusFileSD.OutputFile
	if len(exporter.outputFilename) == 0 {
		return fmt.Errorf("no output filename configured")
	}

	// Create the output directory
	os.MkdirAll(filepath.Dir(exporter.outputFilename), os.ModePerm)

	return nil
}

func (exporter *PrometheusFileSDExporter) UpdateMeshData(meshData *meshdata.MeshData) error {
	if err := exporter.BaseExporter.UpdateMeshData(meshData); err != nil {
		return err
	}

	// Perform exporting the data asynchronously
	go exporter.exportMeshData()
	return nil
}

func (exporter *PrometheusFileSDExporter) exportMeshData() {
	// Data is read, so acquire a read lock
	exporter.locker.RLock()
	defer exporter.locker.RUnlock()

	scrapes := exporter.createScrapeConfigs()
	if err := exporter.exportScrapeConfig(scrapes); err != nil {
		exporter.log.Err(err).Str("file", exporter.outputFilename).Msg("error exporting Prometheus File SD")
	} else {
		exporter.log.Debug().Str("file", exporter.outputFilename).Msg("exported Prometheus File SD")
	}
}

func (exporter *PrometheusFileSDExporter) createScrapeConfigs() []*prometheus.ScrapeConfig {
	var scrapes []*prometheus.ScrapeConfig
	var addScrape = func(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) {
		if scrape := exporter.createScrapeConfig(site, host, endpoint); scrape != nil {
			scrapes = append(scrapes, scrape)
		}
	}

	// Create a scrape config for each service alongside any additional endpoints
	for _, site := range exporter.meshData.Sites {
		for _, service := range site.Services {
			if !service.IsMonitored {
				continue
			}

			// Add the "main" service to the scrapes
			addScrape(site, service.Host, &service.ServiceEndpoint)

			for _, endpoint := range service.AdditionalEndpoints {
				if endpoint.IsMonitored {
					addScrape(site, service.Host, endpoint)
				}
			}
		}
	}

	return scrapes
}

func (exporter *PrometheusFileSDExporter) createScrapeConfig(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig {
	return &prometheus.ScrapeConfig{
		Targets: []string{path.Join(host, endpoint.Path)},
		Labels: map[string]string{
			"site":         site.Name,
			"service-type": endpoint.Type.Name,
		},
	}
}

func (exporter *PrometheusFileSDExporter) exportScrapeConfig(v interface{}) error {
	// Encode scrape config as JSON
	data, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return fmt.Errorf("unable to marshal scrape config: %v", err)
	}

	// Write the data to disk
	if err := ioutil.WriteFile(exporter.outputFilename, data, os.ModePerm); err != nil {
		return fmt.Errorf("unable to write scrape config '%v': %v", exporter.outputFilename, err)
	}

	return nil
}

func (exporter *PrometheusFileSDExporter) GetName() string {
	return "Prometheus File SD"
}

func init() {
	registerExporter(config.ExporterID_PrometheusFileSD, &PrometheusFileSDExporter{})
}
