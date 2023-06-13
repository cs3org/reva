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

package ocsprovider

import (
	"encoding/json"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

func init() {
	global.Register("ocsprovider", New)
}

type config struct {
	WebdavRoot string `mapstructure:"webdav_root" docs:"/remote.php/dav/ocm;The root URL of the WebDAV endpoint to serve OCM shares."`
}

type ocsDiscoveryData struct {
	Version  int         `json:"version"`
	Services ocsServices `json:"services"`
}

type ocsServices struct {
	PrivateData      map[string]any `json:"PRIVATE_DATA"`
	Sharing          map[string]any `json:"SHARING"`
	FederatedSharing map[string]any `json:"FEDERATED_SHARING"`
	Provisioning     map[string]any `json:"PROVISIONING"`
}

type svc struct {
	data *ocsDiscoveryData
}

func (c *config) init() {
	if c.WebdavRoot == "" {
		// same default as for the /ocm-provider discovery service
		c.WebdavRoot = "/remote.php/dav/ocm/"
	}
	if c.WebdavRoot[len(c.WebdavRoot)-1:] != "/" {
		c.WebdavRoot += "/"
	}
}

func (c *config) prepare() *ocsDiscoveryData {
	// generates the (static) data structure to be exposed by /ocs-provider:
	// here we only populate the federated sharing part and leave the rest empty
	var fedSharingData = map[string]any{
		"version": 1,
		"endpoints": map[string]string{
			"webdav": c.WebdavRoot,
		},
	}
	d := &ocsDiscoveryData{}
	d.Version = 2
	d.Services = ocsServices{
		PrivateData:      map[string]any{},
		Sharing:          map[string]any{},
		FederatedSharing: fedSharingData,
		Provisioning:     map[string]any{},
	}
	return d
}

// New returns a new ocsprovider object, that implements
// a minimal OCS discovery endpoint similar to OC10 or NC.
// OCS specs are defined at:
// https://www.freedesktop.org/wiki/Specifications/open-collaboration-services
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.init()
	return &svc{data: conf.prepare()}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	// this is hardcoded as per OCS specifications
	return "/ocs-provider"
}

func (s *svc) Unprotected() []string {
	return []string{"/"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		indented, _ := json.MarshalIndent(s.data, "", "   ")
		if _, err := w.Write(indented); err != nil {
			log.Err(err).Msg("Error writing to ResponseWriter")
		}
	})
}
