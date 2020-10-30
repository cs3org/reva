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

package mentix

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/connectors"
	"github.com/cs3org/reva/pkg/mentix/exchange"
	"github.com/cs3org/reva/pkg/mentix/exchange/exporters"
	"github.com/cs3org/reva/pkg/mentix/exchange/importers"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Mentix represents the main Mentix service object.
type Mentix struct {
	conf *config.Configuration
	log  *zerolog.Logger

	meshData   *meshdata.MeshData
	connectors []connectors.Connector
	importers  []importers.Importer
	exporters  []exporters.Exporter

	updateInterval time.Duration
}

const (
	runLoopSleeptime = time.Millisecond * 500
)

func (mntx *Mentix) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	mntx.conf = conf

	if log == nil {
		return fmt.Errorf("no logger provided")
	}
	mntx.log = log

	// Initialize the connectors that will be used to gather the mesh data
	if err := mntx.initConnectors(); err != nil {
		return fmt.Errorf("unable to initialize connector: %v", err)
	}

	// Initialize the exchangers
	if err := mntx.initExchangers(); err != nil {
		return fmt.Errorf("unable to initialize exchangers: %v", err)
	}

	// Get the update interval
	duration, err := time.ParseDuration(mntx.conf.UpdateInterval)
	if err != nil {
		// If the duration can't be parsed, default to one hour
		duration = time.Hour
	}
	mntx.updateInterval = duration

	// Create empty mesh data
	mntx.meshData = meshdata.New()

	// Log some infos
	connectorNames := make([]string, len(mntx.connectors))
	for idx, connector := range mntx.connectors {
		connectorNames[idx] = connector.GetName()
	}
	importerNames := make([]string, len(mntx.importers))
	for idx, importer := range mntx.importers {
		importerNames[idx] = importer.GetName()
	}
	exporterNames := make([]string, len(mntx.exporters))
	for idx, exporter := range mntx.exporters {
		exporterNames[idx] = exporter.GetName()
	}
	log.Info().Msgf("mentix started with connectors: %v; importers: %v; exporters: %v; update interval: %v", strings.Join(connectorNames, ", "), strings.Join(importerNames, ", "), strings.Join(exporterNames, ", "), duration)

	return nil
}

func (mntx *Mentix) initConnectors() error {
	// Use all connectors exposed by the connectors package
	conns, err := connectors.AvailableConnectors(mntx.conf)
	if err != nil {
		return fmt.Errorf("unable to get registered conns: %v", err)
	}
	mntx.connectors = conns

	// Activate all conns
	for _, connector := range mntx.connectors {
		if err := connector.Activate(mntx.conf, mntx.log); err != nil {
			return fmt.Errorf("unable to activate connector '%v': %v", connector.GetName(), err)
		}
	}

	return nil
}

func (mntx *Mentix) initExchangers() error {
	// Use all importers exposed by the importers package
	imps, err := importers.AvailableImporters(mntx.conf)
	if err != nil {
		return fmt.Errorf("unable to get registered importers: %v", err)
	}
	mntx.importers = imps

	if err := importers.ActivateImporters(mntx.importers, mntx.conf, mntx.log); err != nil {
		return fmt.Errorf("unable to activate importers: %v", err)
	}

	// Use all exporters exposed by the exporters package
	exps, err := exporters.AvailableExporters(mntx.conf)
	if err != nil {
		return fmt.Errorf("unable to get registered exporters: %v", err)
	}
	mntx.exporters = exps

	if err := exporters.ActivateExporters(mntx.exporters, mntx.conf, mntx.log); err != nil {
		return fmt.Errorf("unable to activate exporters: %v", err)
	}

	return nil
}

func (mntx *Mentix) startExchangers() error {
	// Start all importers
	if err := importers.StartImporters(mntx.importers); err != nil {
		return fmt.Errorf("unable to start importers: %v", err)
	}

	// Start all exporters
	if err := exporters.StartExporters(mntx.exporters); err != nil {
		return fmt.Errorf("unable to start exporters: %v", err)
	}

	return nil
}

func (mntx *Mentix) stopExchangers() {
	exporters.StopExporters(mntx.exporters)
	importers.StopImporters(mntx.importers)
}

func (mntx *Mentix) destroy() {
	// Stop all im- & exporters
	mntx.stopExchangers()
}

// Run starts the Mentix service that will periodically pull the configured data source and publish this data
// through the enabled exporters.
func (mntx *Mentix) Run(stopSignal <-chan struct{}) error {
	defer mntx.destroy()

	// Start all im- & exporters; they will be stopped in mntx.destroy
	if err := mntx.startExchangers(); err != nil {
		return fmt.Errorf("unable to start exchangers: %v", err)
	}

	updateTimestamp := time.Time{}
loop:
	for {
		if stopSignal != nil {
			// Poll the stopSignal channel; if a signal was received, break the loop, terminating Mentix gracefully
			select {
			case <-stopSignal:
				break loop

			default:
			}
		}

		// If enough time has passed, retrieve the latest mesh data and update it
		if time.Since(updateTimestamp) >= mntx.updateInterval {
			meshData, err := mntx.retrieveMeshData()
			if err == nil {
				if err := mntx.applyMeshData(meshData); err != nil {
					mntx.log.Err(err).Msg("failed to apply mesh data")
				}
			} else {
				mntx.log.Err(err).Msg("failed to retrieve mesh data")
			}

			updateTimestamp = time.Now()
		}

		time.Sleep(runLoopSleeptime)
	}

	return nil
}

func (mntx *Mentix) retrieveMeshData() (*meshdata.MeshData, error) {
	var mergedMeshData meshdata.MeshData

	for _, connector := range mntx.connectors {
		meshData, err := connector.RetrieveMeshData()
		if err != nil {
			return nil, fmt.Errorf("retrieving mesh data from connector '%v' failed: %v", connector.GetName(), err)
		}
		mergedMeshData.Merge(meshData)
	}

	return &mergedMeshData, nil
}

func (mntx *Mentix) applyMeshData(meshData *meshdata.MeshData) error {
	if !meshData.Compare(mntx.meshData) {
		mntx.log.Debug().Msg("mesh data changed, applying")

		mntx.meshData = meshData

		for _, exporter := range mntx.exporters {
			if err := exporter.UpdateMeshData(meshData); err != nil {
				return fmt.Errorf("unable to update mesh data on exporter '%v': %v", exporter.GetName(), err)
			}
		}
	}

	return nil
}

// GetRequestImporters returns all exporters that can handle HTTP requests.
func (mntx *Mentix) GetRequestImporters() []exchange.RequestExchanger {
	return importers.GetRequestImporters(mntx.importers)
}

// GetRequestExporters returns all exporters that can handle HTTP requests.
func (mntx *Mentix) GetRequestExporters() []exchange.RequestExchanger {
	return exporters.GetRequestExporters(mntx.exporters)
}

// RequestHandler handles any incoming HTTP requests by asking each RequestExchanger whether it wants to
// handle the request (usually based on the relative URL path).
func (mntx *Mentix) RequestHandler(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	switch r.Method {
	case http.MethodGet:
		mntx.handleRequest(mntx.GetRequestExporters(), w, r, log)

	case http.MethodPost:
		mntx.handleRequest(mntx.GetRequestImporters(), w, r, log)

	default:
		log.Err(fmt.Errorf("unsupported method")).Msg("error handling incoming request")
	}
}

func (mntx *Mentix) handleRequest(exchangers []exchange.RequestExchanger, w http.ResponseWriter, r *http.Request, log *zerolog.Logger) {
	// Ask each RequestExchanger if it wants to handle the request
	for _, exporter := range exchangers {
		if exporter.WantsRequest(r) {
			if err := exporter.HandleRequest(w, r); err != nil {
				log.Err(err).Msg("error handling request")
			}
		}
	}
}

// New creates a new Mentix service instance.
func New(conf *config.Configuration, log *zerolog.Logger) (*Mentix, error) {
	mntx := new(Mentix)
	if err := mntx.initialize(conf, log); err != nil {
		return nil, fmt.Errorf("unable to initialize Mentix: %v", err)
	}
	return mntx, nil
}
