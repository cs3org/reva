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

package importers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/exchange"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

const (
	queryMethodDefault = ""
)

type queryCallback func(*meshdata.MeshData, url.Values) ([]byte, error)

// BaseRequestImporter implements basic importer functionality common to all request importers.
type BaseRequestImporter struct {
	BaseImporter
	exchange.BaseRequestExchanger

	defaultMethodHandler queryCallback
}

// HandleRequest handles the actual HTTP request.
func (importer *BaseRequestImporter) HandleRequest(resp http.ResponseWriter, req *http.Request) error {
	data, err := importer.handleQuery(req.URL.Query())
	if err == nil {
		if _, err := resp.Write(data); err != nil {
			return fmt.Errorf("error writing the API request response: %v", err)
		}
	} else {
		return fmt.Errorf("error while serving API request: %v", err)
	}

	return nil
}

func (importer *BaseRequestImporter) handleQuery(params url.Values) ([]byte, error) {
	// Data is written, so lock it completely
	importer.Locker().Lock()
	defer importer.Locker().Unlock()

	method := params.Get("method")
	switch strings.ToLower(method) {
	case queryMethodDefault:
		if importer.defaultMethodHandler != nil {
			return importer.defaultMethodHandler(importer.MeshData(), params)
		}

	default:
		return []byte{}, fmt.Errorf("unknown API method '%v'", method)
	}

	return []byte{}, fmt.Errorf("unhandled query for method '%v'", method)
}
