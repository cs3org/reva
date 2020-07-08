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

package script

import (
	"os"

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
}

// Metrics returns randomly generated values for the defined metrics.
type Metrics struct {
	NumUsersMeasure      *stats.Int64Measure
	NumGroupsMeasure     *stats.Int64Measure
	AmountStorageMeasure *stats.Int64Measure
}

// GetNumUsersView returns the number of site users measure view
func (m *Metrics) GetNumUsersView() *view.View {
	return nil
}

// GetNumGroupsView returns the number of site groups measure view
func (m *Metrics) GetNumGroupsView() *view.View {
	return nil
}

// GetAmountStorageView returns the amount of site storage measure view
func (m *Metrics) GetAmountStorageView() *view.View {
	return nil
}
