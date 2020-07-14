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
	"math/rand"
	"reva/pkg/metrics/config"
)

// New returns a new DummyDataDriver object.
func New(config *config.Config) (*DummyDataDriver, error) {
	driver := &DummyDataDriver{
		config: config,
	}

	return driver, nil
}

// DummyDataDriver the DummyDataDriver struct
type DummyDataDriver struct {
	config *config.Config
}

// GetNumUsers returns the number of site users, it's a dummy number
func (d *DummyDataDriver) GetNumUsers() int64 {
	return int64(rand.Intn(30000))
}

// GetNumGroups returns the number of site groups, it's a dummy number
func (d *DummyDataDriver) GetNumGroups() int64 {
	return int64(rand.Intn(200))
}

// GetAmountStorage returns the amount of site storage used, it's a dummy amount
func (d *DummyDataDriver) GetAmountStorage() int64 {
	return int64(rand.Intn(70000000000))
}
