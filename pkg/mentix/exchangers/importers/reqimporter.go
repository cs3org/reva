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
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/exchangers"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

const (
	queryActionRegisterSite   = "register"
	queryActionUnregisterSite = "unregister"
)

type queryCallback func([]byte, url.Values) (meshdata.Vector, int, []byte, error)

// BaseRequestImporter implements basic importer functionality common to all request importers.
type BaseRequestImporter struct {
	BaseImporter
	exchangers.BaseRequestExchanger

	registerSiteActionHandler   queryCallback
	unregisterSiteActionHandler queryCallback
}

// HandleRequest handles the actual HTTP request.
func (importer *BaseRequestImporter) HandleRequest(resp http.ResponseWriter, req *http.Request) {
	body, _ := ioutil.ReadAll(req.Body)
	meshData, status, respData, err := importer.handleQuery(body, req.URL.Query())
	if err == nil {
		if len(meshData) > 0 {
			importer.mergeImportedMeshData(meshData)
		}
	} else {
		respData = []byte(err.Error())
	}
	resp.WriteHeader(status)
	_, _ = resp.Write(respData)
}

func (importer *BaseRequestImporter) mergeImportedMeshData(meshData meshdata.Vector) {
	// Merge the newly imported data with any existing data stored in the importer
	if importer.meshData != nil {
		// Need to manually lock the data for writing
		importer.Locker().Lock()
		defer importer.Locker().Unlock()

		importer.meshData = append(importer.meshData, meshData...)
	} else {
		importer.SetMeshData(meshData) // SetMeshData will do the locking itself
	}
}

func (importer *BaseRequestImporter) handleQuery(data []byte, params url.Values) (meshdata.Vector, int, []byte, error) {
	action := params.Get("action")
	switch strings.ToLower(action) {
	case queryActionRegisterSite:
		if importer.registerSiteActionHandler != nil {
			return importer.registerSiteActionHandler(data, params)
		}

	case queryActionUnregisterSite:
		if importer.unregisterSiteActionHandler != nil {
			return importer.unregisterSiteActionHandler(data, params)
		}

	default:
		return nil, http.StatusNotImplemented, []byte{}, fmt.Errorf("unknown action '%v'", action)
	}

	return nil, http.StatusNotFound, []byte{}, fmt.Errorf("unhandled query for action '%v'", action)
}
