// Copyright 2018-2021 CERN
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

package accounts

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/accounts/config"
	"github.com/cs3org/reva/pkg/rhttp/global"
)

func init() {
	global.Register(serviceName, New)
}

type svc struct {
	conf *config.Configuration
	log  *zerolog.Logger

	manager *Manager
}

const (
	serviceName = "accounts"
)

// Close is called when this service is being stopped.
func (s *svc) Close() error {
	return nil
}

// Prefix returns the main endpoint of this service.
func (s *svc) Prefix() string {
	return s.conf.Prefix
}

// Unprotected returns all endpoints that can be queried without prior authorization.
func (s *svc) Unprotected() []string {
	// TODO: For testing only
	return []string{"/"}
	// This service currently only has one public endpoint (called "register") used for account registration
	return []string{config.EndpointRegister}
}

// Handler serves all HTTP requests.
func (s *svc) Handler() http.Handler {
	// Allow definition of endpoints in a flexible and easy way
	type Endpoint struct {
		Path    string
		Method  string
		Handler func(url.Values, []byte) (interface{}, error)
	}

	endpoints := []Endpoint{
		{config.EndpointList, http.MethodGet, s.handleList},
		{config.EndpointRegister, http.MethodPost, s.handleRegister},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		// Every request to the accounts service results in a standardized JSON response
		type Response struct {
			Success bool        `json:"success"`
			Error   string      `json:"error,omitempty"`
			Data    interface{} `json:"data,omitempty"`
		}

		// The default response is an unknown endpoint (for the specified method)
		resp := Response{
			Success: false,
			Error:   fmt.Sprintf("unknown endpoint %v for method %v", r.URL.Path, r.Method),
			Data:    nil,
		}

		// Check each endpoint if it can handle the request
		for _, endpoint := range endpoints {
			if r.URL.Path == endpoint.Path && r.Method == endpoint.Method {
				body, _ := ioutil.ReadAll(r.Body)

				if data, err := endpoint.Handler(r.URL.Query(), body); err == nil {
					resp.Success = true
					resp.Error = ""
					resp.Data = data
				} else {
					resp.Success = false
					resp.Error = fmt.Sprintf("%v", err)
					resp.Data = nil
				}

				break
			}
		}

		// Any failure during query handling results in a bad request
		if !resp.Success {
			w.WriteHeader(http.StatusBadRequest)
		}

		jsonData, _ := json.MarshalIndent(&resp, "", "\t")
		_, _ = w.Write(jsonData)
	})
}

func (s *svc) handleList(values url.Values, data []byte) (interface{}, error) {
	return s.manager.ClonedAccounts(), nil
}

func (s *svc) handleRegister(values url.Values, data []byte) (interface{}, error) {
	return map[string]string{"id": "okiii"}, nil
}

func parseConfig(m map[string]interface{}) (*config.Configuration, error) {
	conf := &config.Configuration{}
	if err := mapstructure.Decode(m, &conf); err != nil {
		return nil, errors.Wrap(err, "error decoding configuration")
	}
	applyDefaultConfig(conf)
	return conf, nil
}

func applyDefaultConfig(conf *config.Configuration) {
	if conf.Prefix == "" {
		conf.Prefix = serviceName
	}

	if conf.Storage.Driver == "" {
		conf.Storage.Driver = "file"
	}
}

// New returns a new Accounts service.
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	// Prepare the configuration
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// Create the accounts manager instance
	mngr, err := newManager(conf, log)
	if err != nil {
		return nil, errors.Wrap(err, "error creating the accounts service")
	}

	// Create the service
	s := &svc{
		conf:    conf,
		log:     log,
		manager: mngr,
	}
	return s, nil
}
