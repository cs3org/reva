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

// This package auto registers site metrics in prometheus

import (
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func init() {
	// trigger the actual metric provider functions
	getNumUsers()
	getNumGroups()
	getStorageUsed()
}

func getNumUsers() {
	// here we must request the actual number of users from the site
	// for now this is a mockup: a number increasing over time
	go func() {
		for {
			numUsersCounter.Add(float64(123))
			time.Sleep(2 * time.Second)
		}
	}()
}

var (
	numUsersCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cs3_org_sciencemesh_site_total_num_users",
		Help: "The total number of users within this site",
	})
)

func getNumGroups() {
	// here we must request the actual number of groups from the site
	// for now this is a mockup: a random number changing over time
	rand.Seed(time.Now().UnixNano())
	go func() {
		for {
			numGroupsGauge.Set(float64(rand.Intn(100)))
			time.Sleep(2 * time.Second)
		}
	}()
}

var (
	numGroupsGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cs3_org_sciencemesh_site_total_num_groups",
		Help: "The total number of groups within this site",
	})
)

func getStorageUsed() {
	// here we must request the actual amount of storage used within the site
	// for now this is a mockup: a number increasing over time
	go func() {
		for {
			amountStorageUsed.Add(float64(12345))
			time.Sleep(2 * time.Second)
		}
	}()
}

var (
	amountStorageUsed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "cs3_org_sciencemesh_site_amount_storage_used",
		Help: "The total amount of storage used within this site",
	})
)
