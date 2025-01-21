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

package ldap

import (
	"testing"

	ldapIdentity "github.com/opencloud-eu/reva/v2/pkg/utils/ldap"
)

func TestUserManager(t *testing.T) {
	// negative test for parseConfig
	_, err := New(map[string]interface{}{"uri": 42})
	if err == nil {
		t.Fatal("expected error but got none")
	}
	defaults := ldapIdentity.New()
	internal := map[string]interface{}{
		"mail": "email",
		"dn":   "dn",
	}

	con := map[string]interface{}{
		"user_schema": internal,
	}

	c, err := parseConfig(con)
	if err != nil {
		t.Fatalf("config is invalid")
	}

	// ID not provided in config file. should not modify defaults
	if c.LDAPIdentity.User.Schema.ID != defaults.User.Schema.ID {
		t.Fatalf("expected default ID to be: %v, got %v", defaults.User.Schema.ID, c.LDAPIdentity.User.Schema.ID)
	}

	// DisplayName not provided in config file. should not modify defaults
	if c.LDAPIdentity.User.Schema.DisplayName != defaults.User.Schema.DisplayName {
		t.Fatalf("expected DisplayName to be: %v, got %v", defaults.User.Schema.DisplayName, c.LDAPIdentity.User.Schema.DisplayName)
	}

	// Mail provided in config file
	if c.LDAPIdentity.User.Schema.Mail != "email" {
		t.Fatalf("expected default UID to be: %v, got %v", "email", c.LDAPIdentity.User.Schema.Mail)
	}

	// positive tests for New
	_, err = New(map[string]interface{}{})
	if err != nil {
		t.Fatal(err.Error())
	}
}
