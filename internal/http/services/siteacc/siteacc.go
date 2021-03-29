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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"

	"github.com/cs3org/reva/internal/http/services/siteacc/config"
	"github.com/cs3org/reva/internal/http/services/siteacc/data"
	"github.com/cs3org/reva/pkg/mentix/key"
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
	serviceName = "siteacc"
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
	// The account creation endpoint is always unprotected
	endpoints := []string{config.EndpointCreate}

	// If enabled, the registration registrationForm endpoint is also unprotected
	if s.conf.EnableRegistrationForm {
		endpoints = append(endpoints, config.EndpointRegistration)
	}

	return endpoints
}

// Handler serves all HTTP requests.
func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		switch r.URL.Path {
		case config.EndpointPanel:
			s.handlePanelEndpoint(w, r)

		case config.EndpointRegistration:
			if s.conf.EnableRegistrationForm {
				s.handleRegistrationEndpoint(w, r)
			}

		default:
			s.handleRequestEndpoints(w, r)
		}
	})
}

func (s *svc) handlePanelEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.ShowPanel(w); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the web interface panel: %v", err)))
	}
}

func (s *svc) handleRegistrationEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.ShowRegistrationForm(w); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the web interface registration registrationForm: %v", err)))
	}
}

func (s *svc) handleRequestEndpoints(w http.ResponseWriter, r *http.Request) {
	// Allow definition of endpoints in a flexible and easy way
	type Endpoint struct {
		Path    string
		Method  string
		Handler func(url.Values, []byte) (interface{}, error)
	}

	// Every request to the accounts service results in a standardized JSON response
	type Response struct {
		Success bool        `json:"success"`
		Error   string      `json:"error,omitempty"`
		Data    interface{} `json:"data,omitempty"`
	}

	endpoints := []Endpoint{
		{config.EndpointGenerateAPIKey, http.MethodGet, s.handleGenerateAPIKey},
		{config.EndpointVerifyAPIKey, http.MethodGet, s.handleVerifyAPIKey},
		{config.EndpointAssignAPIKey, http.MethodPost, s.handleAssignAPIKey},
		{config.EndpointList, http.MethodGet, s.handleList},
		{config.EndpointFind, http.MethodGet, s.handleFind},
		{config.EndpointCreate, http.MethodPost, s.handleCreate},
		{config.EndpointUpdate, http.MethodPost, s.handleUpdate},
		{config.EndpointRemove, http.MethodPost, s.handleRemove},
		{config.EndpointAuthorize, http.MethodPost, s.handleAuthorize},
		{config.EndpointIsAuthorized, http.MethodGet, s.handleIsAuthorized},
		{config.EndpointUnregisterSite, http.MethodPost, s.handleUnregisterSite},
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
}

func (s *svc) handleGenerateAPIKey(values url.Values, body []byte) (interface{}, error) {
	email := values.Get("email")
	flags := key.FlagDefault

	if strings.EqualFold(values.Get("isScienceMesh"), "true") {
		flags |= key.FlagScienceMesh
	}

	if len(email) == 0 {
		return nil, errors.Errorf("no email provided")
	}

	apiKey, err := key.GenerateAPIKey(key.SaltFromEmail(email), flags)
	if err != nil {
		return nil, errors.Wrap(err, "unable to generate API key")
	}
	return map[string]string{"apiKey": apiKey}, nil
}

func (s *svc) handleVerifyAPIKey(values url.Values, body []byte) (interface{}, error) {
	apiKey := values.Get("apiKey")
	email := values.Get("email")

	if len(apiKey) == 0 {
		return nil, errors.Errorf("no API key provided")
	}

	if len(email) == 0 {
		return nil, errors.Errorf("no email provided")
	}

	err := key.VerifyAPIKey(apiKey, key.SaltFromEmail(email))
	if err != nil {
		return nil, errors.Wrap(err, "invalid API key")
	}
	return nil, nil
}

func (s *svc) handleAssignAPIKey(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	flags := key.FlagDefault
	if _, ok := values["isScienceMesh"]; ok {
		flags |= key.FlagScienceMesh
	}

	// Assign a new API key to the account through the account manager
	if err := s.manager.AssignAPIKeyToAccount(account, flags); err != nil {
		return nil, errors.Wrap(err, "unable to assign API key")
	}

	return nil, nil
}

func (s *svc) handleList(values url.Values, body []byte) (interface{}, error) {
	return s.manager.CloneAccounts(), nil
}

func (s *svc) handleFind(values url.Values, body []byte) (interface{}, error) {
	account, err := s.findAccount(values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"account": account}, nil
}

func (s *svc) handleCreate(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Create a new account through the account manager
	if err := s.manager.CreateAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to create account")
	}

	return nil, nil
}

func (s *svc) handleUpdate(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Update the account through the account manager; only the basic data of an account can be updated through this endpoint
	if err := s.manager.UpdateAccount(account, false); err != nil {
		return nil, errors.Wrap(err, "unable to update account")
	}

	return nil, nil
}

func (s *svc) handleRemove(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Remove the account through the account manager
	if err := s.manager.RemoveAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to remove account")
	}

	return nil, nil
}

func (s *svc) handleIsAuthorized(values url.Values, body []byte) (interface{}, error) {
	account, err := s.findAccount(values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return account.Data.Authorized, nil
}

func (s *svc) handleUnregisterSite(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Unregister the account's site through the account manager
	if err := s.manager.UnregisterAccountSite(account); err != nil {
		return nil, errors.Wrap(err, "unable to unregister the site of the given account")
	}

	return nil, nil
}

func (s *svc) handleAuthorize(values url.Values, body []byte) (interface{}, error) {
	account, err := s.unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	if val := values.Get("status"); len(val) > 0 {
		var authorize bool
		switch strings.ToLower(val) {
		case "true":
			authorize = true

		case "false":
			authorize = false

		default:
			return nil, errors.Errorf("unsupported authorization status %v", val[0])
		}

		// Authorize the account through the account manager
		if err := s.manager.AuthorizeAccount(account, authorize); err != nil {
			return nil, errors.Wrap(err, "unable to (un)authorize account")
		}
	} else {
		return nil, errors.Errorf("no authorization status provided")
	}

	return nil, nil
}

func (s *svc) unmarshalRequestData(body []byte) (*data.Account, error) {
	account := &data.Account{}
	if err := json.Unmarshal(body, account); err != nil {
		return nil, errors.Wrap(err, "invalid account data")
	}
	return account, nil
}

func (s *svc) findAccount(by string, value string) (*data.Account, error) {
	if len(by) == 0 && len(value) == 0 {
		return nil, errors.Errorf("missing search criteria")
	}

	// Find the account using the account manager
	account, err := s.manager.FindAccount(by, value)
	if err != nil {
		return nil, errors.Wrap(err, "user not found")
	}
	return account, nil
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

// New returns a new Site Accounts service.
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	// Prepare the configuration
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	// Create the accounts manager instance
	mngr, err := newManager(conf, log)
	if err != nil {
		return nil, errors.Wrap(err, "error creating the site accounts service")
	}

	// Create the service
	s := &svc{
		conf:    conf,
		log:     log,
		manager: mngr,
	}
	return s, nil
}
