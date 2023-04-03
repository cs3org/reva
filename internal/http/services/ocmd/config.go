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

type configData struct {
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

type configHandler struct {
	c configData
}

func (h *configHandler) init(c *config) {
	h.c = c.Config
	h.c.Enabled = true
	h.c.APIVersion = "1.1.0"
	if len(c.Prefix) > 0 {
		h.c.Endpoint = fmt.Sprintf("https://%s/%s", c.Host, c.Prefix)
	} else {
		h.c.Endpoint = fmt.Sprintf("https://%s", c.Host)
	}
	h.c.Provider = c.Provider
	if c.Provider == "" {
		h.c.Provider = "reva"
	}
	rtProtos := map[string]string{}
	// webdav is always enabled
	rtProtos["webdav"] = fmt.Sprintf("https://%s/remote.php/dav/%s", c.Host, c.Prefix)
	if c.EnableWebApp {
		rtProtos["webapp"] = fmt.Sprintf("https://%s/external/sciencemesh", c.Host)
	}
	if c.EnableDataTx {
		rtProtos["datatx"] = fmt.Sprintf("https://%s/remote.php/dav/%s", c.Host, c.Prefix)
	}
	h.c.ResourceTypes = []resourceTypes{{
		Name:       "file",           // so far we only support `file`
		ShareTypes: []string{"user"}, // so far we only support `user`
		Protocols:  rtProtos,         // expose the protocols as per configuration
	}}
	// for now we hardcode the capabilities, as this is currently only advisory
	h.c.Capabilities = []string{"/invite-accepted"}
}

// Send sends the configuration to the caller.
func (h *configHandler) Send(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	indentedConf, _ := json.MarshalIndent(h.c, "", "   ")
	if _, err := w.Write(indentedConf); err != nil {
		log.Err(err).Msg("Error writing to ResponseWriter")
	}
}
