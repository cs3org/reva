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

package mentix

import (
	"net/http"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/mentix"
	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/exporters"
	"github.com/cs3org/reva/pkg/rhttp/global"
)

type svc struct {
	conf *config.Configuration
	mntx *mentix.Mentix

	stopSignal chan struct{}
}

const (
	serviceName = "mentix"
)

func (s *svc) Close() error {
	// Trigger and close the stopSignal signal channel to stop Mentix
	s.stopSignal <- struct{}{}
	close(s.stopSignal)

	return nil
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	// Get all endpoints exposed by the RequestExporters
	var endpoints []string
	for _, exporter := range s.mntx.GetRequestExporters() {
		endpoints = append(endpoints, exporter.Endpoint())
	}
	return endpoints
}

func (s *svc) Handler() http.Handler {
	// Forward requests to Mentix
	return http.HandlerFunc(s.mntx.RequestHandler)
}

func (s *svc) startBackgroundService() {
	// Just run Mentix in the background
	go s.mntx.Run(s.stopSignal)
}

func parseConfig(m map[string]interface{}) (*config.Configuration, error) {
	cfg := defaultConfig()
	if err := mapstructure.Decode(m, &cfg); err != nil {
		return nil, errors.Wrap(err, "mentix: error decoding configuration")
	}
	return cfg, nil
}

func defaultConfig() *config.Configuration {
	conf := &config.Configuration{}

	conf.Prefix = serviceName

	conf.Connector = config.ConnectorID_GOCDB          // Use GOCDB
	conf.Exporters = exporters.RegisteredExporterIDs() // Enable all exporters
	conf.UpdateInterval = "1h"                         // Update once per hour

	conf.GOCDB.Scope = "SM" // TODO(Daniel-WWU-IT): This might change in the future
	conf.WebAPI.Endpoint = "/"

	return conf
}

// New returns a new Mentix service
func New(m map[string]interface{}) (global.Service, error) {
	// Prepare the configuration
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// Create the Mentix instance
	mntx, err := mentix.New(conf)
	if err != nil {
		return nil, errors.Wrap(err, "mentix: error creating instance")
	}

	// Create the service and start its background activity
	s := &svc{
		conf:       conf,
		mntx:       mntx,
		stopSignal: make(chan struct{}),
	}
	s.startBackgroundService()
	return s, nil
}

func init() {
	global.Register(serviceName, New)
}
