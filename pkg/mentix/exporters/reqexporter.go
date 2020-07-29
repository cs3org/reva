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
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

const (
	queryMethodDefault = ""
)

type queryCallback func(*meshdata.MeshData, url.Values) ([]byte, error)

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

	defaultMethodHandler queryCallback
}

// Endpoint returns the (relative) endpoint of the exporter.
func (exporter *BaseRequestExporter) Endpoint() string {
	// Ensure that the endpoint starts with a /
	endpoint := exporter.endpoint
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	return strings.TrimSpace(endpoint)
}

// WantsRequest returns whether the exporter wants to handle the incoming request.
func (exporter *BaseRequestExporter) WantsRequest(r *http.Request) bool {
	return r.URL.Path == exporter.Endpoint()
}

// HandleRequest handles the actual HTTP request.
func (exporter *BaseRequestExporter) HandleRequest(resp http.ResponseWriter, req *http.Request) error {
	// Data is read, so acquire a read lock
	exporter.locker.RLock()
	defer exporter.locker.RUnlock()

	data, err := exporter.handleQuery(exporter.meshData, req.URL.Query())
	if err == nil {
		if _, err := resp.Write(data); err != nil {
			return fmt.Errorf("error writing the API request response: %v", err)
		}
	} else {
		return fmt.Errorf("error while serving API request: %v", err)
	}

	return nil
}

func (exporter *BaseRequestExporter) handleQuery(meshData *meshdata.MeshData, params url.Values) ([]byte, error) {
	method := params.Get("method")
	switch strings.ToLower(method) {
	case queryMethodDefault:
		if exporter.defaultMethodHandler != nil {
			return exporter.defaultMethodHandler(meshData, params)
		}

	default:
		return []byte{}, fmt.Errorf("unknown API method '%v'", method)
	}

	return []byte{}, fmt.Errorf("unhandled query for method '%v'", method)
}
