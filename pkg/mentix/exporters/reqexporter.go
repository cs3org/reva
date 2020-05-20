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

// RequestExporter is the interface implemented by exporters that offer an HTTP endpoint.
type RequestExporter interface {
	Exporter

	// Endpoint returns the (relative) endpoint of the exporter.
	Endpoint() string
	// WantsRequest returns whether the exporter wants to handle the incoming request.
	WantsRequest(r *http.Request) bool
	// HandleRequest handles the actual HTTP request.
	HandleRequest(resp http.ResponseWriter, req *http.Request) error
}

// BaseRequestExporter implements basic exporter functionality common to all request exporters.
type BaseRequestExporter struct {
	BaseExporter

	endpoint string
}

func (exporter *BaseRequestExporter) Endpoint() string {
	// Ensure that the endpoint starts with a /
	endpoint := exporter.endpoint
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return strings.TrimSpace(endpoint)
}

func (exporter *BaseRequestExporter) WantsRequest(r *http.Request) bool {
	return r.URL.Path == exporter.Endpoint()
}
