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

package graph

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	msgraph "github.com/yaegashi/msgraph.go/v1.0"
)

func init() {
	registry.Register("graph", New)
}

type manager struct {
	c  *config
	rb *msgraph.GraphServiceRequestBuilder
}

type config struct {
	// BaseURL, eg. `https://graph.microsoft.com/v1.0`
	BaseURL string `mapstructure:"base_uri"`
	// Filter defaults to "userPrincipalName eq '%QUERY%'"
	Filter   string `mapstructure:"filter"`
	Insecure bool   `mapstructure:"insecure"`
	Timeout  int64  `mapstructure:"timeout"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := config{
		Filter:  "userPrincipalName eq '%QUERY%'",
		Timeout: 10,
	}
	if err := mapstructure.Decode(m, &c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}

	c.BaseURL = strings.TrimRight(c.BaseURL, "/")

	return &c, nil
}

// New returns a user manager implementation that uses an ms graph endpoint to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: c.Insecure,
		},
		Proxy: func(r *http.Request) (*url.URL, error) {
			// replace the hardcoded url with our own ...
			n := strings.Replace(r.URL.String(), "https://graph.microsoft.com/v1.0", c.BaseURL, 1)
			// rewrite the url in the request
			if r.URL, err = url.Parse(n); err != nil {
				return nil, err
			}
			return http.ProxyFromEnvironment(r)
		},
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(c.Timeout) * time.Second,
	}
	rb := msgraph.NewClient(client)
	return &manager{
		c:  c,
		rb: rb,
	}, nil
}

func (m *manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	id := uid.GetOpaqueId()
	r := m.rb.Users().ID(id).Request()
	u, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}

	// TODO allow passing on the active flag in the user object

	return m.asCS3User(u), nil
}

func (m *manager) asCS3User(u *msgraph.User) *userpb.User {
	return &userpb.User{
		// we use the graph id as the opaque id, which replaces the oidc sub claim
		// we use the graph base uri as the IdP, which replaces the oidc iss claim
		Id: &userpb.UserId{
			OpaqueId: *u.ID,       // a stable non reassignable id
			Idp:      m.c.BaseURL, // in the scope of this issuer
		},
		Username: *u.UserPrincipalName,
		Mail:     *u.Mail,
		// TODO fallback to other name properties
		DisplayName: *u.DisplayName,
		//Groups:      u.getGroupIDs(),
	}
}

// FinUsers will search for users in the SCLM proevider using a search query
// TODO add pagination support
func (m *manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {

	// TODO check escaping
	escaped := strings.ReplaceAll(strings.ReplaceAll(query, `\`, `\\`), `'`, `\'`)

	r := m.rb.Users().Request()
	filter := strings.ReplaceAll(m.c.Filter, "%QUERY%", escaped)
	r.Filter(filter)
	uc, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}
	users := []*userpb.User{}

	for i := range uc {
		user := m.asCS3User(&uc[i])
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
