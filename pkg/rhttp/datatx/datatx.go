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

// Package datatx provides a library to abstract the complexity
// of using various data transfer protocols.
package datatx

import (
	"net/http"

	"github.com/cs3org/reva/v3/pkg/storage"
)

// DataTX provides an abstraction around various data transfer protocols.
type DataTX interface {
	Handler(fs storage.FS) (http.Handler, error)
}
