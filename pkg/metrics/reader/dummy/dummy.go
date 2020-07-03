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

package dummy

import (
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/metrics"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

func init() {

	log := logger.New().With().Int("pid", os.Getpid()).Logger()

	m := &Metrics{
		NumUsersMeasure:      stats.Int64("cs3_org_sciencemesh_site_total_num_users", "The total number of users within this site", stats.UnitDimensionless),
		NumGroupsMeasure:     stats.Int64("cs3_org_sciencemesh_site_total_num_groups", "The total number of groups within this site", stats.UnitDimensionless),
		AmountStorageMeasure: stats.Int64("cs3_org_sciencemesh_site_total_amount_storage", "The total amount of storage used within this site", stats.UnitBytes),
	}

	// Verify that the struct implements the metrics.Reader interface
	_, ok := interface{}(m).(metrics.Reader)
	if !ok {
		log.Error().Msg("the driver does not implement the metrics.Reader interface")
		return
	}

	// register the desired measures' views
	if err := view.Register(
		m.GetNumUsersView(),
		m.GetNumGroupsView(),
		m.GetAmountStorageView(),
	); err != nil {
		log.Error().Err(err).Msg("error registering views with opencensus exporter")
		return
	}

	// call the actual metric provider functions for the latest metrics every 4th second
	go func() {
		rand.Seed(time.Now().UnixNano())
		for {
			m.getNumUsers()
			m.getNumGroups()
			m.getAmountStorage()
			time.Sleep(4 * time.Second)
		}
	}()
}

// Metrics returns randomly generated values for the defined metrics.
type Metrics struct {
	numUsersCounter      int64
	amountStorageCounter int64

	NumUsersMeasure      *stats.Int64Measure
	NumGroupsMeasure     *stats.Int64Measure
	AmountStorageMeasure *stats.Int64Measure
}

// getNumberUsers links to the underlying number of site users provider
func (m *Metrics) getNumUsers() {
	ctx := context.Background()
	m.numUsersCounter += int64(rand.Intn(100))
	stats.Record(ctx, m.NumUsersMeasure.M(m.numUsersCounter))
}

// GetNumUsersView returns the number of site users measure view
func (m *Metrics) GetNumUsersView() *view.View {
	return &view.View{
		Name:        m.NumUsersMeasure.Name(),
		Description: m.NumUsersMeasure.Description(),
		Measure:     m.NumUsersMeasure,
		Aggregation: view.LastValue(),
	}
}

// getNumberGroups links to the underlying number of site groups provider
func (m *Metrics) getNumGroups() {
	ctx := context.Background()
	var numGroupsCounter = int64(rand.Intn(100))
	stats.Record(ctx, m.NumGroupsMeasure.M(numGroupsCounter))
}

// GetNumGroupsView returns the number of site groups measure view
func (m *Metrics) GetNumGroupsView() *view.View {
	return &view.View{
		Name:        m.NumGroupsMeasure.Name(),
		Description: m.NumGroupsMeasure.Description(),
		Measure:     m.NumGroupsMeasure,
		Aggregation: view.LastValue(),
	}
}

// getAmountStorage links to the underlying amount of storage provider
func (m *Metrics) getAmountStorage() {
	ctx := context.Background()
	m.amountStorageCounter += int64(rand.Intn(12865000))
	stats.Record(ctx, m.AmountStorageMeasure.M(m.amountStorageCounter))
}

// GetAmountStorageView returns the amount of site storage measure view
func (m *Metrics) GetAmountStorageView() *view.View {
	return &view.View{
		Name:        m.AmountStorageMeasure.Name(),
		Description: m.AmountStorageMeasure.Description(),
		Measure:     m.AmountStorageMeasure,
		Aggregation: view.LastValue(),
	}
}
