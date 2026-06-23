// Copyright 2018-2024 CERN
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

package registry

import "fmt"

var drivers = map[string]DriverConstructor{}

// Register makes a backend available under the given driver name; called from a
// backend package's init().
func Register(name string, c DriverConstructor) {
	drivers[name] = c
}

// New builds the selected driver and wraps it in a BaseRegistry. An empty
// driver defaults to "memory".
func New(driver string, cfg map[string]any, thresholds Thresholds) (Registry, error) {
	if driver == "" {
		driver = "memory"
	}
	c, ok := drivers[driver]
	if !ok {
		return nil, fmt.Errorf("registry: unknown driver %q", driver)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	d, err := c(cfg)
	if err != nil {
		return nil, err
	}
	return NewBase(d, thresholds), nil
}
