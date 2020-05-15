// Copyright 2018-2020 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by mentlicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In mentlying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package mentix

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/connectors"
	"github.com/cs3org/reva/pkg/mentix/exporters"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

type Mentix struct {
	conf *config.Configuration

	meshData  *meshdata.MeshData
	connector connectors.Connector
	exporters []exporters.Exporter

	updateInterval time.Duration
}

const (
	runLoopSleeptime = time.Millisecond * 500
)

func (ment *Mentix) initialize(conf *config.Configuration) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	ment.conf = conf

	// Initialize the connector that will be used to gather the mesh data
	if err := ment.initConnector(); err != nil {
		return fmt.Errorf("unable to initialize connector: %v", err)
	}

	// Initialize the exporters
	if err := ment.initExporters(); err != nil {
		return fmt.Errorf("unable to initialize exporters: %v", err)
	}

	// Get the update interval
	duration, err := time.ParseDuration(ment.conf.UpdateInterval)
	if err != nil {
		// If the duration can't be parsed, default to one hour
		duration = time.Hour
	}
	ment.updateInterval = duration

	// Create empty mesh data
	ment.meshData = meshdata.New()

	return nil
}

func (ment *Mentix) initConnector() error {
	// Try to get a connector with the configured ID
	connector, err := connectors.FindConnector(ment.conf.Connector)
	if err != nil {
		return fmt.Errorf("the desired connector could be found: %v", err)
	}
	ment.connector = connector

	// Activate the selected connector
	if err := ment.connector.Activate(ment.conf); err != nil {
		return fmt.Errorf("unable to activate connector: %v", err)
	}

	return nil
}

func (ment *Mentix) initExporters() error {
	// Use all exporters exposed by the exporters package
	exporters, err := exporters.AvailableExporters(ment.conf)
	if err != nil {
		return fmt.Errorf("unable to get registered exporters: %v", err)
	}
	var names []string
	for _, exporter := range exporters {
		names = append(names, exporter.GetName())
	}
	ment.exporters = exporters

	// Activate all exporters
	for _, exporter := range ment.exporters {
		if err := exporter.Activate(ment.conf); err != nil {
			return fmt.Errorf("unable to activate exporter '%v': %v", exporter.GetName(), err)
		}
	}

	return nil
}

func (ment *Mentix) startExporters() error {
	// Start all exporters
	for _, exporter := range ment.exporters {
		if err := exporter.Start(); err != nil {
			return fmt.Errorf("unable to start exporter '%v': %v", exporter.GetName(), err)
		}
	}

	return nil
}

func (ment *Mentix) stopExporters() {
	// Stop all exporters
	for _, exporter := range ment.exporters {
		exporter.Stop()
	}
}

func (ment *Mentix) destroy() {
	// Stop all exporters
	ment.stopExporters()
}

func (ment *Mentix) Run(stopSignal <-chan struct{}) error {
	defer ment.destroy()

	// Start all exporters; they will be stopped in ment.destroy
	if err := ment.startExporters(); err != nil {
		return fmt.Errorf("unable to start exporters: %v", err)
	}

	updateTimestamp := time.Time{}
loop:
	for {
		// Poll the stopSignal channel; if a signal was received, break the loop, terminating Mentix gracefully
		select {
		case <-stopSignal:
			break loop

		default:
		}

		// If enough time has passed, retrieve the latest mesh data and update it
		if time.Since(updateTimestamp) >= ment.updateInterval {
			meshData, err := ment.retrieveMeshData()
			if err == nil {
				if err := ment.applyMeshData(meshData); err != nil {
				}
			} else {
			}

			updateTimestamp = time.Now()
		}

		time.Sleep(runLoopSleeptime)
	}

	return nil
}

func (ment *Mentix) retrieveMeshData() (*meshdata.MeshData, error) {
	meshData, err := ment.connector.RetrieveMeshData()
	if err != nil {
		return nil, fmt.Errorf("retrieving mesh data failed: %v", err)
	}
	return meshData, nil
}

func (ment *Mentix) applyMeshData(meshData *meshdata.MeshData) error {
	if !meshData.Compare(ment.meshData) {
		ment.meshData = meshData

		for _, exporter := range ment.exporters {
			if err := exporter.UpdateMeshData(meshData); err != nil {
				return fmt.Errorf("unable to update mesh data on exporter '%v': %v", exporter.GetName(), err)
			}
		}
	}

	return nil
}

func (ment *Mentix) RequestHandler(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	// Ask each RequestExporter if it wants to handle the request
	for _, exporter := range ment.exporters {
		if reqExporter, ok := exporter.(exporters.RequestExporter); ok {
			if reqExporter.WantsRequest(r) {
				if err := reqExporter.HandleRequest(w, r); err != nil {
					log.Err(err).Msg("error handling request")
				}
			}
		}
	}
}

func New(conf *config.Configuration) (*Mentix, error) {
	ment := new(Mentix)
	if err := ment.initialize(conf); err != nil {
		return nil, fmt.Errorf("unable to initialize Mentix: %v", err)
	}
	return ment, nil
}
