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
	"fmt"
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

func init() {
	fmt.Printf("Init metrics\n")
	getNumUsers()
	getNumGroups()
}

func getNumUsers() {
	// here we must request the actual number of users from the site
	// for now this sets a random dummy value
	rand.Seed(time.Now().UnixNano())
	go func() {
		for {
			numUsersGauge.Set(float64(rand.Intn(10)))
			time.Sleep(2 * time.Second)
		}
	}()
}

var (
	numUsersGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "cs3_org_sciencemesh_site_total_num_users",
		Help: "The total number of users within this site",
	})
)

func getNumGroups() {
	// here we must request the actual number of groups from the site
	// for now this sets a random dummy value
	rand.Seed(time.Now().UnixNano())
	go func() {
		for {
			numGroupsGauge.Set(float64(rand.Intn(10)))
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
