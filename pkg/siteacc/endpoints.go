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

package siteacc

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/pkg/mentix/key"
	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/siteacc/data"
	"github.com/pkg/errors"
)

type methodCallback = func(*Manager, url.Values, []byte) (interface{}, error)

type endpoint struct {
	Path            string
	Handler         func(*Manager, endpoint, http.ResponseWriter, *http.Request)
	MethodCallbacks map[string]methodCallback
	IsPublic        bool
}

func createMethodCallbacks(cbGet methodCallback, cbPost methodCallback) map[string]methodCallback {
	callbacks := make(map[string]methodCallback)

	if cbGet != nil {
		callbacks[http.MethodGet] = cbGet
	}

	if cbPost != nil {
		callbacks[http.MethodPost] = cbPost
	}

	return callbacks
}

func getEndpoints() []endpoint {
	endpoints := []endpoint{
		// Form/panel endpoints
		{config.EndpointAdministration, callAdministrationEndpoint, nil, false},
		{config.EndpointAccount, callAccountEndpoint, nil, true},
		// API key endpoints
		{config.EndpointGenerateAPIKey, callMethodEndpoint, createMethodCallbacks(handleGenerateAPIKey, nil), false},
		{config.EndpointVerifyAPIKey, callMethodEndpoint, createMethodCallbacks(handleVerifyAPIKey, nil), false},
		{config.EndpointAssignAPIKey, callMethodEndpoint, createMethodCallbacks(nil, handleAssignAPIKey), false},
		// General account endpoints
		{config.EndpointList, callMethodEndpoint, createMethodCallbacks(handleList, nil), false},
		{config.EndpointFind, callMethodEndpoint, createMethodCallbacks(handleFind, nil), false},
		{config.EndpointCreate, callMethodEndpoint, createMethodCallbacks(nil, handleCreate), true},
		{config.EndpointUpdate, callMethodEndpoint, createMethodCallbacks(nil, handleUpdate), false},
		{config.EndpointRemove, callMethodEndpoint, createMethodCallbacks(nil, handleRemove), false},
		// Authentication endpoints
		{config.EndpointAuthenticate, callMethodEndpoint, createMethodCallbacks(nil, handleAuthenticate), true},
		// Authorization endpoints
		{config.EndpointAuthorize, callMethodEndpoint, createMethodCallbacks(nil, handleAuthorize), false},
		{config.EndpointIsAuthorized, callMethodEndpoint, createMethodCallbacks(handleIsAuthorized, nil), false},
		// Account site endpoints
		{config.EndpointUnregisterSite, callMethodEndpoint, createMethodCallbacks(nil, handleUnregisterSite), false},
	}

	return endpoints
}

func callAdministrationEndpoint(mngr *Manager, ep endpoint, w http.ResponseWriter, r *http.Request) {
	if err := mngr.ShowAdministrationPanel(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the administration panel: %v", err)))
	}
}

func callAccountEndpoint(mngr *Manager, ep endpoint, w http.ResponseWriter, r *http.Request) {
	if err := mngr.ShowAccountPanel(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the account panel: %v", err)))
	}
}

func callMethodEndpoint(mngr *Manager, ep endpoint, w http.ResponseWriter, r *http.Request) {
	// Every request to the accounts service results in a standardized JSON response
	type Response struct {
		Success bool        `json:"success"`
		Error   string      `json:"error,omitempty"`
		Data    interface{} `json:"data,omitempty"`
	}

	// The default response is an unknown requestHandler (for the specified method)
	resp := Response{
		Success: false,
		Error:   fmt.Sprintf("unknown endpoint %v for method %v", r.URL.Path, r.Method),
		Data:    nil,
	}

	if ep.MethodCallbacks != nil {
		// Search for a matching method in the list of callbacks
		for method, cb := range ep.MethodCallbacks {
			if method == r.Method {
				body, _ := ioutil.ReadAll(r.Body)

				if respData, err := cb(mngr, r.URL.Query(), body); err == nil {
					resp.Success = true
					resp.Error = ""
					resp.Data = respData
				} else {
					resp.Success = false
					resp.Error = fmt.Sprintf("%v", err)
					resp.Data = nil
				}
			}
		}
	}

	// Any failure during query handling results in a bad request
	if !resp.Success {
		w.WriteHeader(http.StatusBadRequest)
	}

	jsonData, _ := json.MarshalIndent(&resp, "", "\t")
	_, _ = w.Write(jsonData)
}

func handleGenerateAPIKey(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
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

func handleVerifyAPIKey(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
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

func handleAssignAPIKey(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	flags := key.FlagDefault
	if _, ok := values["isScienceMesh"]; ok {
		flags |= key.FlagScienceMesh
	}

	// Assign a new API key to the account through the account manager
	if err := mngr.AssignAPIKeyToAccount(account, flags); err != nil {
		return nil, errors.Wrap(err, "unable to assign API key")
	}

	return nil, nil
}

func handleList(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	return mngr.CloneAccounts(true), nil
}

func handleFind(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := findAccount(mngr, values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"account": account.Clone(true)}, nil
}

func handleCreate(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Create a new account through the account manager
	if err := mngr.CreateAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to create account")
	}

	return nil, nil
}

func handleUpdate(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Update the account through the account manager; only the basic data of an account can be updated through this requestHandler
	if err := mngr.UpdateAccount(account, false); err != nil {
		return nil, errors.Wrap(err, "unable to update account")
	}

	return nil, nil
}

func handleRemove(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Remove the account through the account manager
	if err := mngr.RemoveAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to remove account")
	}

	return nil, nil
}

func handleIsAuthorized(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := findAccount(mngr, values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return account.Data.Authorized, nil
}

func handleUnregisterSite(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Unregister the account's site through the account manager
	if err := mngr.UnregisterAccountSite(account); err != nil {
		return nil, errors.Wrap(err, "unable to unregister the site of the given account")
	}

	return nil, nil
}

func handleAuthenticate(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Authenticate the user through the account manager
	if _, err := mngr.AuthenticateUser(account.Email, account.Password.Value); err != nil {
		return nil, errors.Wrap(err, "unable to authenticate user")
	}

	return nil, nil
}

func handleAuthorize(mngr *Manager, values url.Values, body []byte) (interface{}, error) {
	account, err := unmarshalRequestData(body)
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
		if err := mngr.AuthorizeAccount(account, authorize); err != nil {
			return nil, errors.Wrap(err, "unable to (un)authorize account")
		}
	} else {
		return nil, errors.Errorf("no authorization status provided")
	}

	return nil, nil
}

func unmarshalRequestData(body []byte) (*data.Account, error) {
	account := &data.Account{}
	if err := json.Unmarshal(body, account); err != nil {
		return nil, errors.Wrap(err, "invalid account data")
	}
	return account, nil
}

func findAccount(mngr *Manager, by string, value string) (*data.Account, error) {
	if len(by) == 0 && len(value) == 0 {
		return nil, errors.Errorf("missing search criteria")
	}

	// Find the account using the account manager
	account, err := mngr.FindAccount(by, value)
	if err != nil {
		return nil, errors.Wrap(err, "user not found")
	}
	return account, nil
}
