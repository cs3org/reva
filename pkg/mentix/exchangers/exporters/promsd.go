// Copyright 2018-2021 CERN
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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exchangers/exporters/prometheus"
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
	labels := getScrapeTargetLabels(site, host, endpoint)

	// Support both HTTP and HTTPS endpoints by setting the scheme label accordingly
	if len(endpoint.URL) > 0 {
		if url, err := url.Parse(endpoint.URL); err == nil && (url.Scheme == "http" || url.Scheme == "https") {
			labels["__scheme__"] = url.Scheme
		}
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

	// Check if health checks are enabled for the endpoint; if they aren't, skip this endpoint
	if enableHealthChecks := meshdata.GetPropertyValue(endpoint.Properties, meshdata.PropertyEnableHealthChecks, "false"); !strings.EqualFold(enableHealthChecks, "true") {
		return nil
	}

	labels := getScrapeTargetLabels(site, host, endpoint)

	// For health checks, the gRPC port must be set
	if _, ok := labels["__meta_mentix_grpc_port"]; !ok {
		return nil
	}

	return &prometheus.ScrapeConfig{
		Targets: []string{target},
		Labels:  labels,
	}
}

func getScrapeTargetLabels(site *meshdata.Site, host string, endpoint *meshdata.ServiceEndpoint) map[string]string {
	labels := map[string]string{
		"__meta_mentix_site":         site.Name,
		"__meta_mentix_site_type":    meshdata.GetSiteTypeName(site.Type),
		"__meta_mentix_site_id":      site.ID,
		"__meta_mentix_host":         host,
		"__meta_mentix_country":      site.CountryCode,
		"__meta_mentix_service_type": endpoint.Type.Name,
	}

	// Get the gRPC port if the corresponding property has been set
	if port := meshdata.GetPropertyValue(endpoint.Properties, meshdata.PropertyGRPCPort, ""); len(port) > 0 {
		labels["__meta_mentix_grpc_port"] = port
	}

	return labels
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
			if err := os.MkdirAll(filepath.Dir(outputFilename), 0755); err != nil {
				return fmt.Errorf("unable to create output directory tree: %v", err)
			}
		}

		return nil
	}

	// Register all scrape creators
	if err := registerCreator("metrics", conf.Exporters.PrometheusSD.MetricsOutputFile, createMetricsSDScrapeConfig); err != nil {
		return fmt.Errorf("unable to register the 'metrics' scrape config creator: %v", err)
	}

	if err := registerCreator("blackbox", conf.Exporters.PrometheusSD.BlackboxOutputFile, createBlackboxSDScrapeConfig); err != nil {
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
		if err := os.MkdirAll(filepath.Dir(creator.outputFilename), 0755); err != nil {
			return fmt.Errorf("unable to create directory tree: %v", err)
		}
	}

	// Store PrometheusSD specifics
	exporter.SetEnabledConnectors(conf.Exporters.PrometheusSD.EnabledConnectors)

	return nil
}

// Update is called whenever the mesh data set has changed to reflect these changes.
func (exporter *PrometheusSDExporter) Update(meshDataSet meshdata.Map) error {
	if err := exporter.BaseExporter.Update(meshDataSet); err != nil {
		return err
	}

	// Perform exporting the data asynchronously
	go exporter.exportMeshData()
	return nil
}

func (exporter *PrometheusSDExporter) exportMeshData() {
	// Data is read, so acquire a read lock
	exporter.Locker().RLock()
	defer exporter.Locker().RUnlock()

	for name, creator := range exporter.scrapeCreators {
		scrapes := exporter.createScrapeConfigs(creator.creatorCallback)
		if err := exporter.exportScrapeConfig(creator.outputFilename, scrapes); err != nil {
			exporter.Log().Err(err).Str("kind", name).Str("file", creator.outputFilename).Msg("error exporting Prometheus SD scrape config")
		} else {
			exporter.Log().Debug().Str("kind", name).Str("file", creator.outputFilename).Msg("exported Prometheus SD scrape config")
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
	for _, site := range exporter.MeshData().Sites {
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
	if err := ioutil.WriteFile(outputFilename, data, 0755); err != nil {
		return fmt.Errorf("unable to write scrape config '%v': %v", outputFilename, err)
	}

	return nil
}

// GetID returns the ID of the exporter.
func (exporter *PrometheusSDExporter) GetID() string {
	return config.ExporterIDPrometheusSD
}

// GetName returns the display name of the exporter.
func (exporter *PrometheusSDExporter) GetName() string {
	return "Prometheus SD"
}

func init() {
	registerExporter(&PrometheusSDExporter{})
}
