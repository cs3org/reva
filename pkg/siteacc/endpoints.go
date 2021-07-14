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
	"github.com/cs3org/reva/pkg/siteacc/html"
	"github.com/cs3org/reva/pkg/siteacc/manager"
	"github.com/pkg/errors"
)

const (
	invokerDefault = ""
	invokerUser    = "user"
)

type methodCallback = func(*SiteAccounts, url.Values, []byte, *html.Session) (interface{}, error)

type endpoint struct {
	Path            string
	Handler         func(*SiteAccounts, endpoint, http.ResponseWriter, *http.Request, *html.Session)
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
		{config.EndpointUpdate, callMethodEndpoint, createMethodCallbacks(nil, handleUpdate), true},
		{config.EndpointRemove, callMethodEndpoint, createMethodCallbacks(nil, handleRemove), false},
		// Login endpoints
		{config.EndpointLogin, callMethodEndpoint, createMethodCallbacks(nil, handleLogin), true},
		{config.EndpointLogout, callMethodEndpoint, createMethodCallbacks(handleLogout, nil), true},
		{config.EndpointResetPassword, callMethodEndpoint, createMethodCallbacks(nil, handleResetPassword), true},
		// Authorization endpoints
		{config.EndpointAuthorize, callMethodEndpoint, createMethodCallbacks(nil, handleAuthorize), false},
		{config.EndpointIsAuthorized, callMethodEndpoint, createMethodCallbacks(handleIsAuthorized, nil), false},
		// Access management endpoints
		{config.EndpointGrantGOCDBAccess, callMethodEndpoint, createMethodCallbacks(nil, handleGrantGOCDBAccess), false},
		// Account site endpoints
		{config.EndpointUnregisterSite, callMethodEndpoint, createMethodCallbacks(nil, handleUnregisterSite), false},
	}

	return endpoints
}

func callAdministrationEndpoint(siteacc *SiteAccounts, ep endpoint, w http.ResponseWriter, r *http.Request, session *html.Session) {
	if err := siteacc.ShowAdministrationPanel(w, r, session); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the administration panel: %v", err)))
	}
}

func callAccountEndpoint(siteacc *SiteAccounts, ep endpoint, w http.ResponseWriter, r *http.Request, session *html.Session) {
	if err := siteacc.ShowAccountPanel(w, r, session); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("Unable to show the account panel: %v", err)))
	}
}

func callMethodEndpoint(siteacc *SiteAccounts, ep endpoint, w http.ResponseWriter, r *http.Request, session *html.Session) {
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

				if respData, err := cb(siteacc, r.URL.Query(), body, session); err == nil {
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

func handleGenerateAPIKey(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
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

func handleVerifyAPIKey(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
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

func handleAssignAPIKey(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	flags := key.FlagDefault
	if _, ok := values["isScienceMesh"]; ok {
		flags |= key.FlagScienceMesh
	}

	// Assign a new API key to the account through the accounts manager
	if err := siteacc.AccountsManager().AssignAPIKeyToAccount(account, flags); err != nil {
		return nil, errors.Wrap(err, "unable to assign API key")
	}

	return nil, nil
}

func handleList(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	return siteacc.AccountsManager().CloneAccounts(true), nil
}

func handleFind(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := findAccount(siteacc, values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"account": account.Clone(true)}, nil
}

func handleCreate(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Create a new account through the accounts manager
	if err := siteacc.AccountsManager().CreateAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to create account")
	}

	return nil, nil
}

func handleUpdate(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	setPassword := false

	switch strings.ToLower(values.Get("invoker")) {
	case invokerDefault:
		// If the endpoint was called through the API, an API key must be provided identifying the account
		apiKey := values.Get("apikey")
		if apiKey == "" {
			return nil, errors.Errorf("no API key provided")
		}

		accountFound, err := findAccount(siteacc, manager.FindByAPIKey, apiKey)
		if err != nil {
			return nil, errors.Wrap(err, "no account for the specified API key found")
		}
		account.Email = accountFound.Email

	case invokerUser:
		// If this endpoint was called by the user, set the account email from the stored session
		if session.LoggedInUser == nil {
			return nil, errors.Errorf("no user is currently logged in")
		}

		account.Email = session.LoggedInUser.Email
		setPassword = true

	default:
		return nil, errors.Errorf("no invoker provided")
	}

	// Update the account through the accounts manager
	if err := siteacc.AccountsManager().UpdateAccount(account, setPassword, false); err != nil {
		return nil, errors.Wrap(err, "unable to update account")
	}

	return nil, nil
}

func handleRemove(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Remove the account through the accounts manager
	if err := siteacc.AccountsManager().RemoveAccount(account); err != nil {
		return nil, errors.Wrap(err, "unable to remove account")
	}

	return nil, nil
}

func handleIsAuthorized(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := findAccount(siteacc, values.Get("by"), values.Get("value"))
	if err != nil {
		return nil, err
	}
	return account.Data.Authorized, nil
}

func handleUnregisterSite(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Unregister the account's site through the accounts manager
	if err := siteacc.AccountsManager().UnregisterAccountSite(account); err != nil {
		return nil, errors.Wrap(err, "unable to unregister the site of the given account")
	}

	return nil, nil
}

func handleLogin(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Login the user through the users manager
	if err := siteacc.UsersManager().LoginUser(account.Email, account.Password.Value, session); err != nil {
		return nil, errors.Wrap(err, "unable to login user")
	}

	return nil, nil
}

func handleLogout(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	// Logout the user through the users manager
	siteacc.UsersManager().LogoutUser(session)
	return nil, nil
}

func handleResetPassword(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	// Reset the password through the users manager
	if err := siteacc.AccountsManager().ResetPassword(account.Email); err != nil {
		return nil, errors.Wrap(err, "unable to reset password")
	}

	return nil, nil
}

func handleAuthorize(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
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

		// Authorize the account through the accounts manager
		if err := siteacc.AccountsManager().AuthorizeAccount(account, authorize); err != nil {
			return nil, errors.Wrap(err, "unable to (un)authorize account")
		}
	} else {
		return nil, errors.Errorf("no authorization status provided")
	}

	return nil, nil
}

func handleGrantGOCDBAccess(siteacc *SiteAccounts, values url.Values, body []byte, session *html.Session) (interface{}, error) {
	account, err := unmarshalRequestData(body)
	if err != nil {
		return nil, err
	}

	if val := values.Get("status"); len(val) > 0 {
		var grantAccess bool
		switch strings.ToLower(val) {
		case "true":
			grantAccess = true

		case "false":
			grantAccess = false

		default:
			return nil, errors.Errorf("unsupported access status %v", val[0])
		}

		// Grant access to the account through the accounts manager
		if err := siteacc.AccountsManager().GrantGOCDBAccess(account, grantAccess); err != nil {
			return nil, errors.Wrap(err, "unable to change the GOCDB access status of the account")
		}
	} else {
		return nil, errors.Errorf("no access status provided")
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

func findAccount(siteacc *SiteAccounts, by string, value string) (*data.Account, error) {
	if len(by) == 0 && len(value) == 0 {
		return nil, errors.Errorf("missing search criteria")
	}

	// Find the account using the accounts manager
	account, err := siteacc.AccountsManager().FindAccount(by, value)
	if err != nil {
		return nil, errors.Wrap(err, "user not found")
	}
	return account, nil
}
