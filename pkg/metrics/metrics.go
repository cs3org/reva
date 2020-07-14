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

package metrics

import (
	"context"
	"errors"
	"reva/pkg/metrics/config"
	"reva/pkg/metrics/driver/dummy"
	"reva/pkg/metrics/driver/json"

	"github.com/rs/zerolog/log"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

// New returns a Metrics object
func New(c *config.Config) (*Metrics, error) {
	m := &Metrics{
		dataDriverType:       "",
		dataLocation:         "",
		dataDriver:           nil,
		config:               c,
		NumUsersMeasure:      stats.Int64("cs3_org_sciencemesh_site_total_num_users", "The total number of users within this site", stats.UnitDimensionless),
		NumGroupsMeasure:     stats.Int64("cs3_org_sciencemesh_site_total_num_groups", "The total number of groups within this site", stats.UnitDimensionless),
		AmountStorageMeasure: stats.Int64("cs3_org_sciencemesh_site_total_amount_storage", "The total amount of storage used within this site", stats.UnitBytes),
	}

	// register the desired measures' views
	if err := view.Register(
		m.getNumUsersView(),
		m.getNumGroupsView(),
		m.getAmountStorageView(),
	); err != nil {
		log.Error().Err(err).Msg("error registering the driver's views with opencensus exporter")
		return nil, err
	}

	return m, nil
}

// Metrics the metrics struct
type Metrics struct {
	dataDriverType       string
	dataLocation         string
	dataDriver           Reader // the metrics data driver is an implemention of Reader
	config               *config.Config
	NumUsersMeasure      *stats.Int64Measure
	NumGroupsMeasure     *stats.Int64Measure
	AmountStorageMeasure *stats.Int64Measure
}

// RecordMetrics records the latest metrics from the metrics data source as OpenCensus stats views.
func (m *Metrics) RecordMetrics() error {
	if err := initDataDriver(m); err != nil {
		log.Error().Err(err).Msg("Could not set a driver")
		return err
	}
	// record all latest metrics
	m.recordNumUsers()
	m.recordNumGroups()
	m.recordAmountStorage()

	return nil
}

// initDataDriver initializes a data driver and sets it to be the Metrics.dataDriver
func initDataDriver(m *Metrics) error {
	// find out what driver to use
	if m.config.MetricsDataDriverType == "" {
		err := errors.New("Unable to initialize a metrics data driver, has a driver type (metrics_data_driver_type) been configured?")
		return err
	}
	m.dataLocation = m.config.MetricsDataLocation

	// create/init a driver depending on driver type
	if m.config.MetricsDataDriverType == "json" {
		// Because the json metrics data file is only read on json driver creation
		// a json driver must be re-created to make sure we have the current/latest metrics data.
		// Other drivers may need creation only once.
		jsonDriver, err := json.New(m.config)
		if err != nil {
			log.Error().Err(err)
			return err
		}
		m.dataDriver = jsonDriver
		log.Debug().Msgf("Metrics uses json driver")
	}
	if m.config.MetricsDataDriverType == "dummy" && m.dataDriver == nil {
		// the dummy driver does not need to be initialized every time
		dummyDriver, err := dummy.New(m.config)
		if err != nil {
			log.Error().Err(err)
			return err
		}
		m.dataDriver = dummyDriver
		log.Debug().Msgf("Metrics uses dummy driver")
	}
	// no known driver configured, return error
	if m.dataDriver == nil {
		err := errors.New("Unable to initialize a metrics data driver. Has a correct driver type (one of: json, dummy) been configured?")
		return err
	}

	return nil
}

// recordNumUsers records the latest number of site users figure
func (m *Metrics) recordNumUsers() {
	ctx := context.Background()
	stats.Record(ctx, m.NumUsersMeasure.M(m.dataDriver.GetNumUsers()))
}

func (m *Metrics) getNumUsersView() *view.View {
	return &view.View{
		Name:        m.NumUsersMeasure.Name(),
		Description: m.NumUsersMeasure.Description(),
		Measure:     m.NumUsersMeasure,
		Aggregation: view.LastValue(),
	}
}

// recordNumGroups records the latest number of site groups figure
func (m *Metrics) recordNumGroups() {
	ctx := context.Background()
	stats.Record(ctx, m.NumGroupsMeasure.M(m.dataDriver.GetNumGroups()))
}

func (m *Metrics) getNumGroupsView() *view.View {
	return &view.View{
		Name:        m.NumGroupsMeasure.Name(),
		Description: m.NumGroupsMeasure.Description(),
		Measure:     m.NumGroupsMeasure,
		Aggregation: view.LastValue(),
	}
}

// recordAmountStorage records the latest amount storage figure
func (m *Metrics) recordAmountStorage() {
	ctx := context.Background()
	stats.Record(ctx, m.AmountStorageMeasure.M(m.dataDriver.GetAmountStorage()))
}

func (m *Metrics) getAmountStorageView() *view.View {
	return &view.View{
		Name:        m.AmountStorageMeasure.Name(),
		Description: m.AmountStorageMeasure.Description(),
		Measure:     m.AmountStorageMeasure,
		Aggregation: view.LastValue(),
	}
}

// Reader is the interface that defines the metrics to read.
// Any metrics data driver must implement this interface.
// Each function should return the current/latest available metrics figure relevant to that function.
type Reader interface {

	// GetNumUsersView returns an OpenCensus stats view which records the
	// number of users registered in the mesh provider.
	// Metric name: cs3_org_sciencemesh_site_total_num_users
	GetNumUsers() int64

	// GetNumGroupsView returns an OpenCensus stats view which records the
	// number of user groups registered in the mesh provider.
	// Metric name: cs3_org_sciencemesh_site_total_num_groups
	GetNumGroups() int64

	// GetAmountStorageView returns an OpenCensus stats view which records the
	// amount of storage in the system.
	// Metric name: cs3_org_sciencemesh_site_total_amount_storage
	GetAmountStorage() int64
}
