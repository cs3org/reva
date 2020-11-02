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
	queryActionRegisterSite   = "register"
	queryActionUnregisterSite = "unregister"
)

type queryCallback func(url.Values) (*meshdata.MeshData, int, []byte, error)

// BaseRequestImporter implements basic importer functionality common to all request importers.
type BaseRequestImporter struct {
	BaseImporter
	exchange.BaseRequestExchanger

	registerSiteActionHandler   queryCallback
	unregisterSiteActionHandler queryCallback
}

// HandleRequest handles the actual HTTP request.
func (importer *BaseRequestImporter) HandleRequest(resp http.ResponseWriter, req *http.Request) error {
	meshData, status, data, err := importer.handleQuery(req.URL.Query())
	if err == nil {
		importer.mergeImportedMeshData(meshData)

		resp.WriteHeader(status)
		if _, err := resp.Write(data); err != nil {
			return fmt.Errorf("error writing the API request response: %v", err)
		}
	} else {
		return fmt.Errorf("error while serving API request: %v", err)
	}

	return nil
}

func (importer *BaseRequestImporter) mergeImportedMeshData(meshData *meshdata.MeshData) {
	// Data is written, so lock it completely
	importer.Locker().Lock()
	defer importer.Locker().Unlock()

	// Merge the newly imported data with any existing data stored in the importer
	if meshDataOld := importer.MeshData(); meshDataOld != nil {
		meshDataOld.Merge(meshData)
	} else {
		importer.SetMeshData(meshData)
	}
}

func (importer *BaseRequestImporter) handleQuery(params url.Values) (*meshdata.MeshData, int, []byte, error) {
	method := params.Get("action")
	switch strings.ToLower(method) {
	case queryActionRegisterSite:
		if importer.registerSiteActionHandler != nil {
			return importer.registerSiteActionHandler(params)
		}

	case queryActionUnregisterSite:
		if importer.unregisterSiteActionHandler != nil {
			return importer.unregisterSiteActionHandler(params)
		}

	default:
		return nil, http.StatusNotImplemented, []byte{}, fmt.Errorf("unknown action '%v'", method)
	}

	return nil, http.StatusNotFound, []byte{}, fmt.Errorf("unhandled query for action '%v'", method)
}
