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

package meshdirectory

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/meshdirectory"
	"github.com/cs3org/reva/pkg/meshdirectory/manager/registry"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
)

func init() {
	global.Register("meshdirectory", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
	Prefix  string                            `mapstructure:"prefix"`
}

type svc struct {
	mdm  meshdirectory.Manager
	conf *config
}

func getMeshDirManager(c *config) (meshdirectory.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new Mesh Directory HTTP service
func New(m map[string]interface{}) (global.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	if c.Prefix == "" {
		c.Prefix = "meshdir"
	}

	if c.Driver == "" {
		c.Driver = "json"
	}

	mdm, err := getMeshDirManager(c)
	if err != nil {
		return nil, err
	}

	service := &svc{
		conf: c,
		mdm:  mdm,
	}
	return service, nil
}

// Service prefix
func (s *svc) Prefix() string {
	return s.conf.Prefix
}

// Unprotected endpoints
func (s *svc) Unprotected() []string {
	return []string{"/"}
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

// List of enabled Providers
func (s *svc) MeshProviders() []*meshdirectory.MeshProvider {
	return s.mdm.GetMeshProviders()
}

// HTTP service handler
func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		pb, err := json.Marshal(s.MeshProviders())
		if err != nil {
			log.Err(err).Msg("failed to serialize providers to json")
		}
		if _, err := w.Write(pb); err != nil {
			log.Err(err).Msg("error writing response")
		}
	})
}
