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

type prometheusSDScrapeCreatorCallback = func(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig
type prometheusSDScrapeCreator struct {
	outputFilename  string
	creatorCallback prometheusSDScrapeCreatorCallback
}

// PrometheusSDExporter implements various Prometheus Service Discovery scrape config exporters.
type PrometheusSDExporter struct {
	BaseExporter

	scrapeCreators map[string]prometheusSDScrapeCreator
}

func createMetricsSDScrapeConfig(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig {
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

func createBlackboxSDScrapeConfig(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) *prometheus.ScrapeConfig {
	// The URL of the service is used as the actual target; it must be configured properly
	target := endpoint.URL
	if target == "" {
		return nil
	}

	labels := map[string]string{
		"site":         site.Name,
		"country":      site.CountryCode,
		"service_type": endpoint.Type.Name,
	}

	return &prometheus.ScrapeConfig{
		Targets: []string{target},
		Labels:  labels,
	}
}

func (exporter *PrometheusSDExporter) registerScrapeCreators(conf *config.Configuration) error {
	exporter.scrapeCreators = make(map[string]prometheusSDScrapeCreator)

	registerCreator := func(name string, outputFilename string, creator prometheusSDScrapeCreatorCallback) error {
		if len(outputFilename) > 0 { // Only register the creator if an output filename was configured
			exporter.scrapeCreators[name] = prometheusSDScrapeCreator{
				outputFilename:  outputFilename,
				creatorCallback: creator,
			}

			// Create the output directory for the target file so it exists when exporting
			if err := os.MkdirAll(filepath.Dir(outputFilename), os.ModePerm); err != nil {
				return fmt.Errorf("unable to create output directory tree: %v", err)
			}
		}

		return nil
	}

	// Register all scrape creators
	if err := registerCreator("metrics", conf.PrometheusSD.MetricsOutputFile, createMetricsSDScrapeConfig); err != nil {
		return fmt.Errorf("unable to register the 'metrics' scrape config creator: %v", err)
	}

	if err := registerCreator("blackbox", conf.PrometheusSD.BlackboxOutputFile, createBlackboxSDScrapeConfig); err != nil {
		return fmt.Errorf("unable to register the 'blackbox' scrape config creator: %v", err)
	}

	return nil
}

// Activate activates the exporter.
func (exporter *PrometheusSDExporter) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if err := exporter.BaseExporter.Activate(conf, log); err != nil {
		return err
	}

	if err := exporter.registerScrapeCreators(conf); err != nil {
		return fmt.Errorf("unable to register the scrape creators: %v", err)
	}

	// Create all output directories
	for _, creator := range exporter.scrapeCreators {
		if err := os.MkdirAll(filepath.Dir(creator.outputFilename), os.ModePerm); err != nil {
			return fmt.Errorf("unable to create directory tree: %v", err)
		}
	}

	return nil
}

// UpdateMeshData is called whenever the mesh data has changed to reflect these changes.
func (exporter *PrometheusSDExporter) UpdateMeshData(meshData *meshdata.MeshData) error {
	if err := exporter.BaseExporter.UpdateMeshData(meshData); err != nil {
		return err
	}

	// Perform exporting the data asynchronously
	go exporter.exportMeshData()
	return nil
}

func (exporter *PrometheusSDExporter) exportMeshData() {
	// Data is read, so acquire a read lock
	exporter.locker.RLock()
	defer exporter.locker.RUnlock()

	for name, creator := range exporter.scrapeCreators {
		scrapes := exporter.createScrapeConfigs(creator.creatorCallback)
		if err := exporter.exportScrapeConfig(creator.outputFilename, scrapes); err != nil {
			exporter.log.Err(err).Str("kind", name).Str("file", creator.outputFilename).Msg("error exporting Prometheus SD scrape config")
		} else {
			exporter.log.Debug().Str("kind", name).Str("file", creator.outputFilename).Msg("exported Prometheus SD scrape config")
		}
	}
}

func (exporter *PrometheusSDExporter) createScrapeConfigs(creatorCallback prometheusSDScrapeCreatorCallback) []*prometheus.ScrapeConfig {
	var scrapes []*prometheus.ScrapeConfig
	var addScrape = func(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) {
		if scrape := creatorCallback(site, host, endpoint); scrape != nil {
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

func (exporter *PrometheusSDExporter) exportScrapeConfig(outputFilename string, v interface{}) error {
	// Encode scrape config as JSON
	data, err := json.MarshalIndent(v, "", "\t")
	if err != nil {
		return fmt.Errorf("unable to marshal scrape config: %v", err)
	}

	// Write the data to disk
	if err := ioutil.WriteFile(outputFilename, data, os.ModePerm); err != nil {
		return fmt.Errorf("unable to write scrape config '%v': %v", outputFilename, err)
	}

	return nil
}

// GetName returns the display name of the exporter.
func (exporter *PrometheusSDExporter) GetName() string {
	return "PrometheusSD SD"
}

func init() {
	registerExporter(config.ExporterIDPrometheusSD, &PrometheusSDExporter{})
}
