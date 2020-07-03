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
	"go.opencensus.io/stats/view"
)

// Reader is the interface that defines how the metrics will be read.
type Reader interface {

	// GetNumUsersView returns an OpenCensus stats view which records the
	// number of users registered in the mesh provider.
	GetNumUsersView() *view.View

	// GetNumGroupsView returns an OpenCensus stats view which records the
	// number of user groups registered in the mesh provider.
	GetNumGroupsView() *view.View

	// GetAmountStorageView returns an OpenCensus stats view which records the
	// amount of storage in the system.
	GetAmountStorageView() *view.View
}
