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

package exporters

import (
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/cs3org/reva/pkg/mentix/exchangers"
)

// BaseRequestExporter implements basic exporter functionality common to all request exporters.
type BaseRequestExporter struct {
	BaseExporter
	exchangers.BaseRequestExchanger
}

// HandleRequest handles the actual HTTP request.
func (exporter *BaseRequestExporter) HandleRequest(resp http.ResponseWriter, req *http.Request) {
	body, _ := ioutil.ReadAll(req.Body)
	status, respData, err := exporter.handleQuery(body, req.URL.Query())
	if err != nil {
		respData = []byte(err.Error())
	}
	resp.WriteHeader(status)
	_, _ = resp.Write(respData)
}

func (exporter *BaseRequestExporter) handleQuery(body []byte, params url.Values) (int, []byte, error) {
	// Data is read, so lock it for writing
	exporter.Locker().RLock()
	defer exporter.Locker().RUnlock()

	_, status, data, err := exporter.HandleAction(exporter.MeshData(), body, params)
	return status, data, err
}
