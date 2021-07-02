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

package siteacc

import (
	"fmt"
	"net/http"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// SiteAccounts represents the main Site Accounts service object.
type SiteAccounts struct {
	conf *config.Configuration
	log  *zerolog.Logger

	manager *Manager
}

func (siteacc *SiteAccounts) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	siteacc.conf = conf

	if log == nil {
		return fmt.Errorf("no logger provided")
	}
	siteacc.log = log

	// Create the accounts manager instance
	mngr, err := newManager(conf, log)
	if err != nil {
		return errors.Wrap(err, "error creating the site accounts manager")
	}
	siteacc.manager = mngr

	return nil
}

// RequestHandler returns the HTTP request handler of the service.
func (siteacc *SiteAccounts) RequestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		epHandled := false
		for _, ep := range getEndpoints() {
			if ep.Path == r.URL.Path {
				ep.Handler(siteacc.manager, ep, w, r)
				epHandled = true
				break
			}
		}

		if !epHandled {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("Unknown endpoint %v", r.URL.Path)))
		}
	})
}

func (siteacc *SiteAccounts) GetPublicEndpoints() []string {
	// TODO: REMOVE!
	return []string{"/"}

	endpoints := make([]string, 0, 5)
	for _, ep := range getEndpoints() {
		if ep.IsPublic {
			endpoints = append(endpoints, ep.Path)
		}
	}
	return endpoints
}

// New returns a new Site Accounts service instance.
func New(conf *config.Configuration, log *zerolog.Logger) (*SiteAccounts, error) {
	// Configure the accounts service
	siteacc := new(SiteAccounts)
	if err := siteacc.initialize(conf, log); err != nil {
		return nil, fmt.Errorf("unable to initialize SiteAccounts: %v", err)
	}
	return siteacc, nil
}
