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

import "github.com/cs3org/reva/v3/pkg/auth"

// NewTokenFunc is the function that token strategies
// should register at init time.
type NewTokenFunc func(map[string]interface{}) (auth.TokenStrategy, error)

// NewTokenFuncs is a map containing all the registered auth strategies.
var NewTokenFuncs = map[string]NewTokenFunc{}

// Register registers a new auth strategy  new function.
// Not safe for concurrent use. Safe for use from package init.
func Register(name string, f NewTokenFunc) {
	NewTokenFuncs[name] = f
}
