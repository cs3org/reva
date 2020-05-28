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

// This package defines site metrics measures and views based on opencensus.io

import (
	"context"
	"math/rand"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
)

func init() {
	// call the actual metric provider functions for the latest metrics every 4th second
	go func() {
		rand.Seed(time.Now().UnixNano())
		for {
			getNumUsers()
			getNumGroups()
			getAmountStorage()
			time.Sleep(4 * time.Second)
		}
	}()
}

// Create the measures
var (
	NumUsersMeasure      = stats.Int64("cs3_org_sciencemesh_site_total_num_users", "The total number of users within this site", stats.UnitDimensionless)
	NumGroupsMeasure     = stats.Int64("cs3_org_sciencemesh_site_total_num_groups", "The total number of groups within this site", stats.UnitDimensionless)
	AmountStorageMeasure = stats.Int64("cs3_org_sciencemesh_site_total_amount_storage", "The total amount of storage used within this site", stats.UnitBytes)
)

// initialize local dummy counters
var (
	numUsersCounter      = int64(0)
	amountStorageCounter = int64(0)
)

// getNumberUsers links to the underlying number of site users provider
func getNumUsers() {
	ctx := context.Background()
	// here we must request the actual number of site users
	// for now this is a mockup: a number increasing over time
	numUsersCounter += int64(rand.Intn(100))
	stats.Record(ctx, NumUsersMeasure.M(numUsersCounter))
}

// GetNumUsersView returns the number of site users measure view
func GetNumUsersView() *view.View {
	return &view.View{
		Name:        NumUsersMeasure.Name(),
		Description: NumUsersMeasure.Description(),
		Measure:     NumUsersMeasure,
		Aggregation: view.LastValue(),
	}
}

// getNumberGroups links to the underlying number of site groups provider
func getNumGroups() {
	ctx := context.Background()
	// here we must request the actual number of site groups
	// for now this is a mockup: a number changing over time
	var numGroupsCounter = int64(rand.Intn(100))
	stats.Record(ctx, NumGroupsMeasure.M(numGroupsCounter))
}

// GetNumGroupsView returns the number of site groups measure view
func GetNumGroupsView() *view.View {
	return &view.View{
		Name:        NumGroupsMeasure.Name(),
		Description: NumGroupsMeasure.Description(),
		Measure:     NumGroupsMeasure,
		Aggregation: view.LastValue(),
	}
}

// getAmountStorage links to the underlying amount of storage provider
func getAmountStorage() {
	ctx := context.Background()
	// here we must request the actual amount of storage used
	// for now this is a mockup: a number increasing over time
	amountStorageCounter += int64(rand.Intn(12865000))
	stats.Record(ctx, AmountStorageMeasure.M(amountStorageCounter))
}

// GetAmountStorageView returns the amount of site storage measure view
func GetAmountStorageView() *view.View {
	return &view.View{
		Name:        AmountStorageMeasure.Name(),
		Description: AmountStorageMeasure.Description(),
		Measure:     AmountStorageMeasure,
		Aggregation: view.LastValue(),
	}
}
