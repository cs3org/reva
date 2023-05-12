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

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	conversions "github.com/cs3org/reva/pkg/cbox/utils"
	"github.com/cs3org/reva/pkg/ocm/invite"
	"github.com/cs3org/reva/pkg/ocm/invite/repository/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// This module implement the invite.Repository interface as an api(call with external API) driver.

func init() {
	registry.Register("api", New)
}

// Client is an API client.
type Client struct {
	Config    *config
	HTTPClient *http.Client
}

type config struct {
	BaseURL string `mapstructure:"base_url"`
	ApiKey string `mapstructure:"api_key"`
}

type apiToken struct {
	Token     string `json:"token"`
	Initiator string `json:"initiator"`
	Description string `json:"description"`
	Expiration time.Time `json:"expiry_date"`
}

type apiOCMUser struct {
	OpaqueUserID string `json:"opaqueUserId"`
	Idp          string `json:"idp"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
}

// New returns a new invite manager object.
func New(m map[string]interface{}) (invite.Repository, error) {
	config, err := parseConfig(m)
	if err != nil {
		return nil, errors.Wrap(err, "error parsing config for api invite repository")
	}

	client := &Client{
		Config: config,
		HTTPClient:   &http.Client{},
	}

	return client, nil
}

func (c *Client) init() {
	if c.Config.BaseURL == "" {
		c.Config.BaseURL = "http://localhost/"
	}
}

func parseConfig(c map[string]interface{}) (*config, error) {
	var conf config
	if err := mapstructure.Decode(c, &conf); err != nil {
		return nil, err
	}
	return &conf, nil
}

func normalizeDomain(d string) (string, error) {
	var urlString string
	if strings.Contains(d, "://") {
		urlString = d
	} else {
		urlString = "https://" + d
	}

	u, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	return u.Hostname(), nil
}

func timestampToTime(t *types.Timestamp) time.Time {
	return time.Unix(int64(t.Seconds), int64(t.Nanos))
}

func convertToInviteToken(tkn *apiToken) *invitepb.InviteToken {
	return &invitepb.InviteToken{
		Token:  tkn.Token,
		UserId: conversions.ExtractUserID(tkn.Initiator),
		Expiration: &types.Timestamp{
			Seconds: uint64(tkn.Expiration.Unix()),
		},
		Description: tkn.Description,
	}
}

func (u *apiOCMUser) toCS3User() *userpb.User {
	return &userpb.User{
		Id: &userpb.UserId{
			Idp:      u.Idp,
			OpaqueId: u.OpaqueUserID,
			Type:     userpb.UserType_USER_TYPE_FEDERATED,
		},
		Mail:        u.Email,
		DisplayName: u.DisplayName,
	}
}

func (c *Client) doPostToken(token string, initiator string, description string, expiration time.Time) (bool, error) {
	bodyObj := &apiToken{
		Token:     token,
		Initiator: initiator,
		Description:description,
		Expiration:expiration,
	}

	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return false, err
	}

	requestUrl := c.Config.BaseURL + "/api/v1/add_token/" + initiator

	req, err := http.NewRequest(http.MethodPost, requestUrl, strings.NewReader(string(bodyStr)))
	if err != nil {
		return false, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return false, fmt.Errorf("Unexpected response code from EFSS API: " + strconv.Itoa(resp.StatusCode))
	}
	return true, nil
}

func (c *Client) doGetToken(token string) (*apiToken, error) {
	requestUrl := c.Config.BaseURL + "/api/v1/get_token"  + "?token=" + token
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected response code from API: " + strconv.Itoa(resp.StatusCode))
	}

	result := &apiToken{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) doGetAllTokens(initiator string) ([]*apiToken, error) {
	requestUrl := c.Config.BaseURL + "/api/v1/tokens_list/" + initiator
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected response code from API: " + strconv.Itoa(resp.StatusCode))
	}

	result := []*apiToken{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) doPostRemoteUser(initiator string, opaque_user_id string, idp string, email string, display_name string) (bool, error) {
	bodyObj := &apiOCMUser{
		DisplayName:     display_name,
		Email: email,
		Idp:idp,
		OpaqueUserID:opaque_user_id,
	}

	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return false, err
	}
	requestUrl := c.Config.BaseURL + "/api/v1/add_remote_user/" + initiator
	req, err := http.NewRequest(http.MethodPost, requestUrl, strings.NewReader(string(bodyStr)))
	if err != nil {
		return false, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return false, fmt.Errorf("Unexpected response code from EFSS API: " + strconv.Itoa(resp.StatusCode))
	}
	return true, nil
}

func (c *Client) doGetRemoteUser(initiator string, opaque_user_id string, idp string) (*apiOCMUser, error) {
	requestUrl := c.Config.BaseURL + "/api/v1/get_remote_user/" + initiator + "?userId=" + opaque_user_id + "&idp=" + idp
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected response code from API: " + strconv.Itoa(resp.StatusCode))
	}

	result := &apiOCMUser{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) doGetAllRemoteUsers(initiator string, search string) ([]*apiOCMUser, error) {
	requestUrl := c.Config.BaseURL + "/api/v1/find_remote_user/" + initiator + "?search=" + search
	req, err := http.NewRequest(http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.Config.ApiKey)

	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected response code from API: " + strconv.Itoa(resp.StatusCode))
	}

	result := []*apiOCMUser{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	return result, nil
}


// AddToken stores the token in the external repository.
func (c *Client) AddToken(ctx context.Context, token *invitepb.InviteToken) error {
	result , err := c.doPostToken(token.Token, conversions.FormatUserID(token.UserId), token.Description, timestampToTime(token.Expiration))
	if result != true {
		return err
	}
	return nil
}

// GetToken gets the token from the external repository.
func (c *Client) GetToken(ctx context.Context, token string) (*invitepb.InviteToken, error) {
	t, err := c.doGetToken(token)
	if err != nil{
		return nil, err
	}
	return convertToInviteToken(t), nil
}

func (c *Client) ListTokens(ctx context.Context, initiator *userpb.UserId) ([]*invitepb.InviteToken, error) {
	tokens := []*invitepb.InviteToken{}
	rows, err := c.doGetAllTokens(conversions.FormatUserID(initiator))
	if err != nil {
		return nil, err
	}

	for _, row := range rows{
		tokens = append(tokens, convertToInviteToken(row))
	}

	return tokens, nil
}

// AddRemoteUser stores the remote user.
func (c *Client) AddRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUser *userpb.User) error {
	if _, err := c.doPostRemoteUser(conversions.FormatUserID(initiator), conversions.FormatUserID(remoteUser.Id), remoteUser.Id.Idp, remoteUser.Mail, remoteUser.DisplayName); err != nil {
		return err
	}
	return nil
}

// GetRemoteUser retrieves details about a remote user who has accepted an invite to share.
func (c *Client) GetRemoteUser(ctx context.Context, initiator *userpb.UserId, remoteUserID *userpb.UserId) (*userpb.User, error) {
	result, err := c.doGetRemoteUser(conversions.FormatUserID(initiator), conversions.FormatUserID(remoteUserID), remoteUserID.Idp)
	if err != nil{
		return nil, err
	}
	return result.toCS3User(), nil
}

// FindRemoteUsers finds remote users who have accepted invites based on their attributes.
func (c *Client) FindRemoteUsers(ctx context.Context, initiator *userpb.UserId, attr string) ([]*userpb.User, error) {
	rows, err := c.doGetAllRemoteUsers(conversions.FormatUserID(initiator), attr)
	if err != nil{
		return nil, err
	}

	result := []*userpb.User{}

	for _, row := range rows{
		result = append(result, row.toCS3User())
	}

	return result, nil
}
