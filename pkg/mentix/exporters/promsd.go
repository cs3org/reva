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
	"path/filepath"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exporters/prometheus"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

type prometheusSDExporterScrapeCreator = func(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig

type prometheusSDExporter struct {
	BaseExporter

	exporterName  string
	scrapeCreator prometheusSDExporterScrapeCreator

	outputFilename string
}

// Activate activates the exporter.
func (exporter *prometheusSDExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := exporter.BaseExporter.Activate(conf, log); err != nil {
		return err
	}

	// Check the exporter settings (must be filled out properly in derived classes)
	if len(exporter.outputFilename) == 0 {
		return fmt.Errorf("no output filename configured")
	}

	// Create the output directory
	if err := os.MkdirAll(filepath.Dir(exporter.outputFilename), os.ModePerm); err != nil {
		return fmt.Errorf("unable to create directory tree")
	}

	return nil
}

// UpdateMeshData is called whenever the mesh data has changed to reflect these changes.
func (exporter *prometheusSDExporter) UpdateMeshData(meshData *meshdata.MeshData) error {
	if err := exporter.BaseExporter.UpdateMeshData(meshData); err != nil {
		return err
	}

	// Perform exporting the data asynchronously
	go exporter.exportMeshData()
	return nil
}

func (exporter *prometheusSDExporter) exportMeshData() {
	// Data is read, so acquire a read lock
	exporter.locker.RLock()
	defer exporter.locker.RUnlock()

	scrapes := exporter.createScrapeConfigs()
	if err := exporter.exportScrapeConfig(scrapes); err != nil {
		exporter.log.Err(err).Str("file", exporter.outputFilename).Msg("error exporting " + exporter.exporterName)
	} else {
		exporter.log.Debug().Str("file", exporter.outputFilename).Msg("exported " + exporter.exporterName)
	}
}

func (exporter *prometheusSDExporter) createScrapeConfigs() []*prometheus.ScrapeConfig {
	var scrapes []*prometheus.ScrapeConfig
	var addScrape = func(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) {
		if scrape := exporter.scrapeCreator(site, host, endpoint); scrape != nil {
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
			addScrape(site, service.Host, service.ServiceEndpoint)

			// Add all additional endpoints as well
			for _, endpoint := range service.AdditionalEndpoints {
				if endpoint.IsMonitored {
					addScrape(site, service.Host, endpoint)
				}
			}
		}
	}

	return scrapes
}

func (exporter *prometheusSDExporter) exportScrapeConfig(v interface{}) error {
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

// GetName returns the display name of the exporter.
func (exporter *prometheusSDExporter) GetName() string {
	return exporter.exporterName
}

func initPrometheusSDExporter(exporter *prometheusSDExporter, name string, scrapeCreator prometheusSDExporterScrapeCreator) {
	exporter.exporterName = name
	exporter.scrapeCreator = scrapeCreator
}
