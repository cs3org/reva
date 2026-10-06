// Copyright 2018-2024 CERN
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

package wellknown

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

const OCMAPIVersion = "1.3.0"

type OcmProviderConfig struct {
	OCMPrefix          string `docs:"ocm;The prefix URL where the OCM API is served."                                            mapstructure:"ocm_prefix"`
	Endpoint           string `docs:"This host's full URL. If it's not configured, it is assumed OCM is not available."          mapstructure:"endpoint"`
	Provider           string `docs:"reva;A friendly name that defines this service."                                            mapstructure:"provider"`
	WebdavRoot         string `docs:"/remote.php/dav/ocm;The root URL of the WebDAV endpoint to serve OCM shares."               mapstructure:"webdav_root"`
	InviteAcceptDialog string `docs:"/open-cloud-mesh/accept-invite;The frontend URL where to land when receiving an invitation" mapstructure:"invite_accept_dialog"`
	EnableWebapp       bool   `docs:"false;Whether web apps are enabled in OCM shares."                                          mapstructure:"enable_webapp"`
	EnableEmbedded     bool   `docs:"false;Whether embedded shares are enabled in OCM shares."                                   mapstructure:"enable_embedded"`
	EnableCodeFlow     bool   `docs:"false;Whether code-flow token exchange is enabled in OCM shares."                           mapstructure:"enable_code_flow"`
}

type OcmDiscoveryData struct {
	Enabled            bool            `json:"enabled"            xml:"enabled"`
	APIVersion         string          `json:"apiVersion"         xml:"apiVersion"`
	Endpoint           string          `json:"endPoint"           xml:"endPoint"`
	Provider           string          `json:"provider"           xml:"provider"`
	ResourceTypes      []ResourceTypes `json:"resourceTypes"      xml:"resourceTypes"`
	Capabilities       []string        `json:"capabilities"       xml:"capabilities"`
	Criteria           []string        `json:"criteria"           xml:"criteria"`
	InviteAcceptDialog string          `json:"inviteAcceptDialog" xml:"inviteAcceptDialog"`
	TokenEndPoint      string          `json:"tokenEndPoint,omitempty" xml:"tokenEndPoint,omitempty"`
}

type ResourceTypes struct {
	Name       string         `json:"name"`
	ShareTypes []string       `json:"shareTypes"`
	Protocols  map[string]any `json:"protocols"`
}

type wkocmHandler struct {
	data *OcmDiscoveryData
}

func (c *OcmProviderConfig) ApplyDefaults() {
	if c.OCMPrefix == "" {
		c.OCMPrefix = "ocm"
	}
	if c.Provider == "" {
		c.Provider = "reva"
	}
	if c.WebdavRoot == "" {
		c.WebdavRoot = "/remote.php/dav/ocm/"
	}
	if c.WebdavRoot[len(c.WebdavRoot)-1:] != "/" {
		c.WebdavRoot += "/"
	}
	if c.InviteAcceptDialog == "" {
		c.InviteAcceptDialog = "/open-cloud-mesh/accept-invite"
	}
}

func (h *wkocmHandler) init(c *OcmProviderConfig) {
	// generates the (static) data structure to be exposed by /.well-known/ocm:
	// first prepare an empty and disabled payload
	c.ApplyDefaults()
	d := &OcmDiscoveryData{}
	d.Enabled = false
	d.Endpoint = ""
	d.APIVersion = OCMAPIVersion
	d.Provider = c.Provider
	d.ResourceTypes = []ResourceTypes{{
		Name:       "file",
		ShareTypes: []string{},
		Protocols:  map[string]any{},
	}}
	d.Capabilities = []string{}

	if c.Endpoint == "" {
		h.data = d
		return
	}

	endpointURL, err := url.Parse(c.Endpoint)
	if err != nil {
		h.data = d
		return
	}

	// now prepare the enabled one
	d.Enabled = true
	d.Endpoint, _ = url.JoinPath(c.Endpoint, c.OCMPrefix)
	rtProtos := map[string]any{}
	// webdav is always enabled and we support receiving absolute URIs
	rtProtos["webdav"] = filepath.Join(endpointURL.Path, c.WebdavRoot)
	rtProtos["webdav-receive"] = map[string]any{
		"uri": "absolute",
	}
	if c.EnableWebapp {
		// if webapps are enabled, we can both send and receive webapp shares
		rtProtos["webapp"] = map[string]any{}
		rtProtos["webapp-receive"] = map[string]any{
			"targets": []string{"blank"},
		}
	}
	d.ResourceTypes = []ResourceTypes{
		{
			Name:       "file",
			ShareTypes: []string{"user"}, // so far we only support `user`
			Protocols:  rtProtos,         // expose the protocols as per configuration
		},
		{
			Name:       "folder", // same as file
			ShareTypes: []string{"user"},
			Protocols:  rtProtos,
		},
	}
	if c.EnableEmbedded {
		// declare that we are able to receive ro-crate shares (sending is not implemented)
		d.ResourceTypes = append(d.ResourceTypes, ResourceTypes{
			Name:       "ro-crate",
			ShareTypes: []string{"user"},
			Protocols: map[string]any{
				"embedded-receive": map[string]any{},
			},
		})
	}

	// expose the enabled capabilities
	d.Capabilities = []string{"invites", "protocol-object", "invite-wayf"}
	d.Criteria = []string{"must-invite"}
	d.InviteAcceptDialog, _ = url.JoinPath(c.Endpoint, c.InviteAcceptDialog)
	if c.EnableCodeFlow {
		d.TokenEndPoint, _ = TokenEndpoint(c.Endpoint, c.OCMPrefix)
		d.Capabilities = append(d.Capabilities, "exchange-token")
	}
	h.data = d
}

// TokenEndpoint builds the advertised code-flow token endpoint for OCM discovery.
func TokenEndpoint(baseURL, prefix string) (string, error) {
	return url.JoinPath(baseURL, prefix, "token")
}

// Ocm handles the OCM discovery endpoint specified in
// https://cs3org.github.io/OCM-API/docs.html?repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
func (h *wkocmHandler) Ocm(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.UserAgent() == "Nextcloud Server Crawler" {
		// Nextcloud decided to only support OCM 1.0 and 1.1, not any 1.x as per SemVer. See
		// https://github.com/nextcloud/server/pull/39574#issuecomment-1679191188
		h.data.APIVersion = "1.1"
	} else {
		h.data.APIVersion = OCMAPIVersion
	}
	indented, _ := json.MarshalIndent(h.data, "", "   ")
	if _, err := w.Write(indented); err != nil {
		log.Err(err).Msg("Error writing to ResponseWriter")
	}
}
