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

package exporters

import (
	"net/http"
	"strings"
)

type RequestExporter interface {
	Exporter

	WantsRequest(r *http.Request) bool
	HandleRequest(resp http.ResponseWriter, req *http.Request) error
}

type BaseRequestExporter struct {
	BaseExporter

	endpoint string
}

func (exporter *BaseRequestExporter) WantsRequest(r *http.Request) bool {
	// Make sure that the endpoint starts with a /
	endpoint := exporter.endpoint
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	return r.URL.Path == endpoint
}
