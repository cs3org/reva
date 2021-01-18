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

package exchangers

import (
	"net/http"
	"strings"
)

// RequestExchanger is the interface implemented by exchangers that offer an HTTP endpoint.
type RequestExchanger interface {
	// Endpoint returns the (relative) endpoint of the exchanger.
	Endpoint() string
	// IsProtectedEndpoint returns true if the endpoint can only be accessed with authorization.
	IsProtectedEndpoint() bool
	// WantsRequest returns whether the exchanger wants to handle the incoming request.
	WantsRequest(r *http.Request) bool
	// HandleRequest handles the actual HTTP request.
	HandleRequest(resp http.ResponseWriter, req *http.Request)
}

// BaseRequestExchanger implements basic exporter functionality common to all request exporters.
type BaseRequestExchanger struct {
	RequestExchanger

	endpoint            string
	isProtectedEndpoint bool
}

// Endpoint returns the (relative) endpoint of the exchanger.
func (exchanger *BaseRequestExchanger) Endpoint() string {
	// Ensure that the endpoint starts with a /
	endpoint := exchanger.endpoint
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return strings.TrimSpace(endpoint)
}

// IsProtectedEndpoint returns true if the endpoint can only be accessed with authorization.
func (exchanger *BaseRequestExchanger) IsProtectedEndpoint() bool {
	return exchanger.isProtectedEndpoint
}

// SetEndpoint sets the (relative) endpoint of the exchanger.
func (exchanger *BaseRequestExchanger) SetEndpoint(endpoint string, isProtected bool) {
	exchanger.endpoint = endpoint
	exchanger.isProtectedEndpoint = isProtected
}

// WantsRequest returns whether the exchanger wants to handle the incoming request.
func (exchanger *BaseRequestExchanger) WantsRequest(r *http.Request) bool {
	return r.URL.Path == exchanger.Endpoint()
}

// HandleRequest handles the actual HTTP request.
func (exchanger *BaseRequestExchanger) HandleRequest(resp http.ResponseWriter, req *http.Request) error {
	return nil
}
