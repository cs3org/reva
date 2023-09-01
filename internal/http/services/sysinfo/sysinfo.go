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

package sysinfo

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/cs3org/reva/pkg/sysinfo"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	rhttp.Register(serviceName, New)
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = serviceName
	}
}

type svc struct {
	conf *config
}

const (
	serviceName = "sysinfo"
)

func (s *svc) Name() string {
	return serviceName
}

func (s *svc) Register(r mux.Router) {
	r.Get("/sysinfo", mux.HandlerFunc(func(w http.ResponseWriter, r *http.Request, _ mux.Params) {
		log := appctx.GetLogger(r.Context())
		if _, err := w.Write([]byte(s.getJSONData())); err != nil {
			log.Err(err).Msg("error writing SysInfo response")
		}
	}))
}

func (s *svc) Unprotected() []string {
	return []string{"/sysinfo"}
}

// Close is called when this service is being stopped.
func (s *svc) Close() error {
	return nil
}

// Prefix returns the main endpoint of this service.
func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) getJSONData() string {
	if data, err := sysinfo.SysInfo.ToJSON(); err == nil {
		return data
	}

	return ""
}

// New returns a new SysInfo service.
func New(ctx context.Context, m map[string]interface{}) (rhttp.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// Create the service
	s := &svc{
		conf: &c,
	}
	return s, nil
}
