// Copyright 2018-2019 CERN
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

package scim

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("scim", New)
}

type manager struct {
	c               *config
	client          *http.Client
	usersBase       string
	usersBaseSearch string
}

type config struct {
	// BaseURI, eg. `https://example.com/v2/`, see https://tools.ietf.org/html/rfc7644#section-1.3
	BaseURI string `mapstructure:"base_uri"`
	// Authorization can be one of `basic`, `bearer`, `header`.
	// The spec leaves this up to the implementation, see https://tools.ietf.org/html/rfc7644#section-2
	Authorization string `mapstructure:"authorization"`
	// Username is sent when using `basic` auth
	Username string `mapstructure:"username"`
	// Secret is sent
	// as password when using `basic` auth,
	// as token when using `bearer` auth and
	// as value when using `header` auth.
	Secret string `mapstructure:"secret"`
	// Header is the header to set when using `header` based auth
	Header string `mapstructure:"header"`
	// MediaType defaults to `application/scim+json`.
	// Legacy scim providers might use `application/json`, see https://tools.ietf.org/html/rfc7644#section-8.1
	MediaType string `mapstructure:"media_type"`
	Insecure  bool   `mapstructure:"insecure"`
	Timeout   int64  `mapstructure:"timeout"`

	// Filter used when searching users. Defaults to `displayName co "%QUERY%" OR userName co "%QUERY%" or emails.value co "%QUERY%"`.
	// %s is replaced with the escaped query (`"` -> `\"`)
	// To only match exact emails use `emails.value eq "%QUERY%"`.
	// See https://tools.ietf.org/html/rfc7644#section-3.4.2.2
	Filter string `mapstructure:"filter"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := config{
		MediaType: "application/scim+json",
		Timeout:   10,
		Filter:    `displayName co "%QUERY%" OR userName co "%QUERY%" or emails.value co "%QUERY%"`,
	}
	if err := mapstructure.Decode(m, &c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	c.Authorization = strings.ToLower(c.Authorization)

	return &c, nil
}

// New returns a user manager implementation that connects to a LDAP server to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.Insecure,
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(c.Timeout) * time.Second,
	}
	return &manager{
		c:               c,
		client:          client,
		usersBase:       strings.TrimRight(c.BaseURI, "/") + "/Users",
		usersBaseSearch: strings.TrimRight(c.BaseURI, "/") + "/Users/.search",
	}, nil
}

func (m *manager) setAuth(r *http.Request) {
	switch m.c.Authorization {
	case "basic":
		r.SetBasicAuth(m.c.Username, m.c.Secret)
	case "bearer":
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", m.c.Secret))
	case "header":
		r.Header.Set(m.c.Header, m.c.Secret)
	}
}

// Email is an address for the user, see https://tools.ietf.org/html/rfc7643#page-53
type Email struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary"`
	// Type indicates the attribute's function, e.g., 'work' or 'home'.
	Type string `json:"type"`
}

// Name holds the components of the user's real name, see https://tools.ietf.org/html/rfc7643#page-48
type Name struct {
	Formatted       string `json:"formatted"`
	FamilyName      string `json:"familyName"`
	GivenName       string `json:"givenName"`
	MiddleName      string `json:"middleName"`
	HonorificPrefix string `json:"honorificPrefix"`
	HonorificSuffix string `json:"honorificSuffix"`
}

// User is the resource descriping a user, see https://tools.ietf.org/html/rfc7643#section-8.7.1
type User struct {
	ID string `json:"id"`
	// UserType identifies the relationship between the organization and the user.  Typical values used might be 'Contractor', 'Employee', 'Intern', 'Temp', 'External', and 'Unknown', but any value may be used.
	// TODO "Guest" or "Share" ?
	UserType    string   `json:"userType"`
	UserName    string   `json:"userName"`
	Name        *Name    `json:"name"`
	DisplayName string   `json:"displayName"`
	NickName    string   `json:"nickName"`
	Emails      []*Email `json:"emails"`
	Active      bool     `json:"active"`
	Groups      []*Group `json:"groups"`
	// TODO photos
	// TODO roles
	// TODO entitlements
}

func (u User) getGroupIDs() []string {
	gs := make([]string, len(u.Groups))
	for i := range u.Groups {
		gs = append(gs, u.Groups[i].Value)
	}
	return gs
}

// Group to which the user belongs, either through direct membership, through nested groups, or dynamically calculated.
type Group struct {
	// Value is the identifier of the User's group.
	Value string `json:"value"`
	// Reference is the URI of the corresponding 'Group' resource to which the user belongs.
	Reference string `json:"$ref"`
	// Display is a human-readable name, primarily used for display purposes.  READ-ONLY.
	Display string `json:"display"`
	// Type indicates the attribute's function, e.g., 'direct' or 'indirect'.
	Type string `json:"type"`
}

func (u User) getPrimaryMail() string {
	for i := range u.Emails {
		if u.Emails[i].Primary {
			return u.Emails[i].Value
		}
	}
	// fallback to first entry ... sort maybe? The primary flag should be good enough.
	if len(u.Emails) > 0 {
		return u.Emails[0].Value
	}
	return ""
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)
	// TODO use scim_id from user object
	id := uid.GetOpaqueId()
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/%s", m.usersBase, id), nil)
	if err != nil {
		return nil, err
	}
	//req.Header.Set("Content-Type", m.c.MediaType)
	req.Header.Set("Accept", m.c.MediaType)

	m.setAuth(req)

	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("body", string(body)).Msg("body")
	u := &User{}
	err = json.Unmarshal(body, &u)
	if err != nil {
		return nil, err
	}

	// TODO allow passing on the an active flag in the user object

	return m.asCS3User(u), nil
}

func (m *manager) asCS3User(u *User) *userpb.User {
	return &userpb.User{
		// we use the scim id as the opaque id, which replaces the oidc sub claim
		// we use the scim base uri as the IdP, which replaces the oidc iss claim
		Id: &userpb.UserId{
			OpaqueId: u.ID,        // a stable non reassignable id
			Idp:      m.c.BaseURI, // in the scope of this issuer
		},
		Username: u.UserName,
		Mail:     u.getPrimaryMail(),
		// TODO fallback to other name properties
		DisplayName: u.DisplayName,
		Groups:      u.getGroupIDs(),
	}
}

// SearchRequest is sent as the json body when searching for users
//	   {
//	        "schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
//	        "attributes": ["displayName", "userName"],
//	        "filter":
//	          "displayName sw \"%QUERY%\"",
//	        "startIndex": 1,
//	        "count": 10
//	   }
type SearchRequest struct {
	Schemas    []string `json:"schemas"`
	Attributes []string `json:"attributes"`
	Filter     string   `json:"filter"`
	StartIndex int64    `json:"startIndex,omitempty"` // If not given defaults to 1
	Count      int64    `json:"count,omitempty"`      // If not given, count is determined by the server
}

// ListResponse is the json response for a search request
type ListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int64    `json:"totalResults"`
	ItemsPerPage int64    `json:"itemsPerPage"`
	StartIndex   int64    `json:"startIndex"`
	// we limit search to the /Users resource, so we only get user resources
	Resources []*User `json:"Resources"`
}

// FinUsers will search for users in the SCLM proevider using a search query
// TODO add pagination support
func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	log := appctx.GetLogger(ctx)

	// we need to escape the query because it will be placed inside json formatted string. See https://tools.ietf.org/html/rfc7159#section-7
	escaped := strings.ReplaceAll(strings.ReplaceAll(query, `\`, `\\`), `"`, `\"`)
	filter := strings.ReplaceAll(m.c.Filter, "%QUERY%", escaped)

	sr := SearchRequest{
		Schemas:    []string{"urn:ietf:params:scim:api:messages:2.0:SearchRequest"},
		Attributes: []string{"displayName", "userName", "emails", "groups"}, // id is always returned
		Filter:     filter,
	}
	js, err := json.Marshal(sr)

	req, err := http.NewRequest("POST", m.usersBaseSearch, bytes.NewBuffer(js))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", m.c.MediaType)
	req.Header.Set("Accept", m.c.MediaType)

	m.setAuth(req)

	res, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("body", string(body)).Msg("body")
	lr := &ListResponse{}
	err = json.Unmarshal(body, &lr)
	if err != nil {
		return nil, err
	}

	users := []*userpb.User{}

	for i := range lr.Resources {
		user := m.asCS3User(lr.Resources[i])
		users = append(users, user)
	}

	return users, nil
}

func (m *manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	return []string{}, nil // FIXME implement GetUserGroups for scim user manager
}

func (m *manager) IsInGroup(ctx context.Context, uid *userpb.UserId, group string) (bool, error) {
	return false, nil // FIXME implement IsInGroup for scim user manager
}
