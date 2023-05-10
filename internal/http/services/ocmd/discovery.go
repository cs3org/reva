// Copyright 2018-2023 CERN
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

package ocmd

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
)

type discoveryData struct {
	Enabled       bool            `json:"enabled" xml:"enabled"`
	APIVersion    string          `json:"apiVersion" xml:"apiVersion"`
	Endpoint      string          `json:"endPoint" xml:"endPoint"`
	Provider      string          `json:"provider" xml:"provider"`
	ResourceTypes []resourceTypes `json:"resourceTypes" xml:"resourceTypes"`
	Capabilities  []string        `json:"capabilities" xml:"capabilities"`
}

type resourceTypes struct {
	Name       string            `json:"name"`
	ShareTypes []string          `json:"shareTypes"`
	Protocols  map[string]string `json:"protocols"`
}

type discoHandler struct {
	d discoveryData
}

func (h *discoHandler) init(c *config) {
	h.d.Enabled = true
	h.d.APIVersion = "1.1.0"
	h.d.Endpoint = fmt.Sprintf("%s/%s", c.Endpoint, c.Prefix)
	h.d.Provider = c.Provider
	rtProtos := map[string]string{}
	// webdav is always enabled
	rtProtos["webdav"] = fmt.Sprintf("%s/%s/%s", c.Endpoint, c.WebDAVRoot, c.Prefix)
	if c.EnableWebApp {
		rtProtos["webapp"] = fmt.Sprintf("%s/%s", c.Endpoint, c.WebAppRoot)
	}
	if c.EnableDataTx {
		rtProtos["datatx"] = fmt.Sprintf("%s/%s/%s", c.Endpoint, c.WebDAVRoot, c.Prefix)
	}
	h.d.ResourceTypes = []resourceTypes{{
		Name:       "file",           // so far we only support `file`
		ShareTypes: []string{"user"}, // so far we only support `user`
		Protocols:  rtProtos,         // expose the protocols as per configuration
	}}
	// for now we hardcode the capabilities, as this is currently only advisory
	h.d.Capabilities = []string{"/invite-accepted"}
}

// Send sends the discovery info to the caller.
func (h *discoHandler) Send(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	indentedConf, _ := json.MarshalIndent(h.d, "", "   ")
	if _, err := w.Write(indentedConf); err != nil {
		log.Err(err).Msg("Error writing to ResponseWriter")
	}
}
