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

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

func init() {
	registry.Register("rest", New)
}

var (
	emailRegex    = regexp.MustCompile(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)
	usernameRegex = regexp.MustCompile(`^[ a-zA-Z0-9.-_]+$`)
)

type manager struct {
	conf                *config
	sync.Mutex          // concurrent access to the file and loaded
	apiToken            string
	tokenExpirationTime time.Time
}

type config struct {
	// Base API Endpoint
	APIBaseURL string `mapstructure:"api_base_url" docs:"https://authorization-service-api-dev.web.cern.ch/api/v1.0"`
	// Client ID needed to authenticate
	ClientID string `mapstructure:"client_id" docs:"-"`
	// Client Secret
	ClientSecret string `mapstructure:"client_secret" docs:"-"`

	// Endpoint to generate token to access the API
	OIDCTokenEndpoint string `mapstructure:"oidc_token_endpoint" docs:"https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"`
	// The target application for which token needs to be generated
	TargetAPI string `mapstructure:"target_api" docs:"authorization-service-api"`

	// The OIDC Provider
	IDProvider string `mapstructure:"id_provider" docs:"http://cernbox.cern.ch"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns a user manager implementation that makes calls to the GRAPPA API.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	if c.APIBaseURL == "" {
		c.APIBaseURL = "https://authorization-service-api-dev.web.cern.ch/api/v1.0"
	}

	if c.TargetAPI == "" {
		c.TargetAPI = "authorization-service-api"
	}

	if c.OIDCTokenEndpoint == "" {
		c.OIDCTokenEndpoint = "https://keycloak-dev.cern.ch/auth/realms/cern/api-access/token"
	}

	if c.IDProvider == "" {
		c.IDProvider = "http://cernbox.cern.ch"
	}

	return &manager{
		conf: c,
	}, nil
}

func (m *manager) renewAPIToken(ctx context.Context) error {
	// Received tokens have an expiration time of 20 minutes.
	// Take a couple of seconds as buffer time for the API call to complete
	if m.tokenExpirationTime.Before(time.Now().Add(time.Second * time.Duration(2))) {
		token, expiration, err := m.getAPIToken(ctx)
		if err != nil {
			return err
		}

		m.Lock()
		defer m.Unlock()

		m.apiToken = token
		m.tokenExpirationTime = expiration
	}
	return nil
}

func (m *manager) getAPIToken(ctx context.Context) (string, time.Time, error) {

	params := url.Values{
		"grant_type": {"client_credentials"},
		"audience":   {m.conf.TargetAPI},
	}

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "POST", m.conf.OIDCTokenEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	httpReq.SetBasicAuth(m.conf.ClientID, m.conf.ClientSecret)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return "", time.Time{}, err
	}

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return "", time.Time{}, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", time.Time{}, err
	}

	expirationSecs := result["expires_in"].(float64)
	expirationTime := time.Now().Add(time.Second * time.Duration(expirationSecs))
	return result["access_token"].(string), expirationTime, nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {

	err := m.renewAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/Identity/?filter=id:%s&field=upn&field=primaryAccountEmail&field=displayName", m.conf.APIBaseURL, uid.OpaqueId)

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// We don't need to take the lock when reading apiToken, because if we reach here,
	// the token is valid at least for a couple of seconds. Even if another request modifies
	// the token and expiration time while this request is in progress, the current token will still be valid.
	httpReq.Header.Set("Authorization", "Bearer "+m.apiToken)

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	userData, ok := result["data"].([]interface{})[0].(map[string]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	userGroups, err := m.GetUserGroups(ctx, uid)
	if err != nil {
		return nil, err
	}

	return &userpb.User{
		Id:          uid,
		Username:    userData["upn"].(string),
		Mail:        userData["primaryAccountEmail"].(string),
		DisplayName: userData["displayName"].(string),
		Groups:      userGroups,
	}, nil

}

func (m *manager) findUsersByFilter(ctx context.Context, url string) ([]*userpb.User, error) {

	err := m.renewAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.apiToken)

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	userData, ok := result["data"].([]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	users := []*userpb.User{}

	for _, usr := range userData {
		usrInfo, ok := usr.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}

		uid := &userpb.UserId{
			OpaqueId: usrInfo["id"].(string),
			Idp:      m.conf.IDProvider,
		}

		userGroups, err := m.GetUserGroups(ctx, uid)
		if err != nil {
			return nil, err
		}
		users = append(users, &userpb.User{
			Id:          uid,
			Username:    usrInfo["upn"].(string),
			Mail:        usrInfo["primaryAccountEmail"].(string),
			DisplayName: usrInfo["displayName"].(string),
			Groups:      userGroups,
		})
	}

	return users, nil
}

func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {

	var filters []string
	switch {
	case usernameRegex.MatchString(query):
		filters = []string{"upn", "displayName", "primaryAccountEmail"}
	case emailRegex.MatchString(query):
		filters = []string{"primaryAccountEmail"}
	default:
		return nil, errors.New("rest: illegal characters present in query")
	}

	users := []*userpb.User{}

	for _, f := range filters {
		url := fmt.Sprintf("%s/Identity/?filter=%s:contains:%s&field=id&field=upn&field=primaryAccountEmail&field=displayName", m.conf.APIBaseURL, f, query)
		filteredUsers, err := m.findUsersByFilter(ctx, url)
		if err != nil {
			return nil, err
		}
		users = append(users, filteredUsers...)
	}
	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {

	err := m.renewAPIToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/Identity/%s/groups", m.conf.APIBaseURL, uid.OpaqueId)

	httpClient := rhttp.GetHTTPClient(ctx)
	httpReq, err := rhttp.NewRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.apiToken)

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	groupData, ok := result["data"].([]interface{})
	if !ok {
		return nil, errors.New("rest: error in type assertion")
	}

	groups := make([]string, len(groupData))

	for _, g := range groupData {
		groupInfo, ok := g.(map[string]interface{})
		if !ok {
			return nil, errors.New("rest: error in type assertion")
		}

		groups = append(groups, groupInfo["displayName"].(string))
	}

	return groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, uid *userpb.UserId, group string) (bool, error) {
	userGroups, err := m.GetUserGroups(ctx, uid)
	if err != nil {
		return false, err
	}

	for _, g := range userGroups {
		if group == g {
			return true, nil
		}
	}
	return false, nil
}
