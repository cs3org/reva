// Copyright 2018-2021 CERN
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

import "github.com/cs3org/reva/pkg/auth"

// NewFunc is the function that auth registry implementations
// should register at init time.
type NewFunc func(map[string]interface{}) (auth.Registry, error)

// NewFuncs is a map containing all the registered auth registries.
var NewFuncs = map[string]NewFunc{}

// Register registers a new auth registry new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewFunc) {
	NewFuncs[name] = f
}
