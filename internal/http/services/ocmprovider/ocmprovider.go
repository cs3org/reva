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

package ocmprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

const OCMAPIVersion = "1.1.0"

func init() {
	global.Register("ocmprovider", New)
}

type config struct {
	OCMPrefix    string `mapstructure:"ocm_prefix" docs:"ocm;The prefix URL where the OCM API is served."`
	Endpoint     string `mapstructure:"endpoint" docs:"This host's full URL. If it's not configured, it is assumed OCM is not available."`
	Provider     string `mapstructure:"provider" docs:"reva;A friendly name that defines this service."`
	WebdavRoot   string `mapstructure:"webdav_root" docs:"/remote.php/dav/ocm;The root URL of the WebDAV endpoint to serve OCM shares."`
	WebappRoot   string `mapstructure:"webapp_root" docs:"/external/sciencemesh;The root URL to serve Web apps via OCM."`
	EnableWebapp bool   `mapstructure:"enable_webapp" docs:"false;Whether web apps are enabled in OCM shares."`
	EnableDatatx bool   `mapstructure:"enable_datatx" docs:"false;Whether data transfers are enabled in OCM shares."`
}

type DiscoveryData struct {
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

type svc struct {
	data *DiscoveryData
}

func (c *config) ApplyDefaults() {
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
	if c.WebappRoot == "" {
		c.WebappRoot = "/external/sciencemesh/"
	}
	if c.WebappRoot[len(c.WebappRoot)-1:] != "/" {
		c.WebappRoot += "/"
	}
}

func (c *config) prepare() *DiscoveryData {
	// generates the (static) data structure to be exposed by /ocm-provider:
	// first prepare an empty and disabled payload
	d := &DiscoveryData{}
	d.Enabled = false
	d.Endpoint = ""
	d.APIVersion = OCMAPIVersion
	d.Provider = c.Provider
	d.ResourceTypes = []resourceTypes{{
		Name:       "file",
		ShareTypes: []string{},
		Protocols:  map[string]string{},
	}}
	d.Capabilities = []string{}

	if c.Endpoint == "" {
		return d
	}

	endpointURL, err := url.Parse(c.Endpoint)
	if err != nil {
		return d
	}

	// now prepare the enabled one
	d.Enabled = true
	d.Endpoint, _ = url.JoinPath(c.Endpoint, c.OCMPrefix)
	rtProtos := map[string]string{}
	// webdav is always enabled
	rtProtos["webdav"] = filepath.Join(endpointURL.Path, c.WebdavRoot)
	if c.EnableWebapp {
		rtProtos["webapp"] = filepath.Join(endpointURL.Path, c.WebappRoot)
	}
	if c.EnableDatatx {
		rtProtos["datatx"] = filepath.Join(endpointURL.Path, c.WebdavRoot)
	}
	d.ResourceTypes = []resourceTypes{{
		Name:       "file",           // so far we only support `file`
		ShareTypes: []string{"user"}, // so far we only support `user`
		Protocols:  rtProtos,         // expose the protocols as per configuration
	}}
	// for now we hardcode the capabilities, as this is currently only advisory
	d.Capabilities = []string{"/invite-accepted"}
	return d
}

// New returns a new ocmprovider object, that implements
// the OCM discovery endpoint specified in
// https://cs3org.github.io/OCM-API/docs.html?repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return &svc{data: c.prepare()}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	// this is hardcoded as per OCM specifications
	return "/ocm-provider"
}

func (s *svc) Unprotected() []string {
	return []string{"/"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.UserAgent() == "Nextcloud Server Crawler" {
			// Nextcloud decided to only support OCM 1.0 and 1.1, not any 1.x as per SemVer. See
			// https://github.com/nextcloud/server/pull/39574#issuecomment-1679191188
			s.data.APIVersion = "1.1"
		} else {
			s.data.APIVersion = OCMAPIVersion
		}
		indented, _ := json.MarshalIndent(s.data, "", "   ")
		if _, err := w.Write(indented); err != nil {
			log.Err(err).Msg("Error writing to ResponseWriter")
		}
	})
}
