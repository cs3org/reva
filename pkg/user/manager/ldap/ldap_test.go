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
)

func TestUserManager(t *testing.T) {
	// negative test for parseConfig
	_, err := New(map[string]interface{}{"hostname": 42})
	if err == nil {
		t.Fatal("expected error but got none")
	}

	internal := map[string]interface{}{
		"mail": "email",
		"dn":   "dn",
	}

	con := map[string]interface{}{
		"schema": internal,
	}

	c, err := parseConfig(con)
	if err != nil {
		t.Fatalf("config is invalid")
	}

	// UID not provided in config file. should not modify defaults
	if c.Schema.UID != ldapDefaults.UID {
		t.Fatalf("expected default UID to be: %v, got %v", ldapDefaults.UID, c.Schema.UID)
	}

	// DisplayName not provided in config file. should not modify defaults
	if c.Schema.DisplayName != ldapDefaults.DisplayName {
		t.Fatalf("expected DisplayName to be: %v, got %v", ldapDefaults.DisplayName, c.Schema.DisplayName)
	}

	// Mail provided in config file
	if c.Schema.Mail != "email" {
		t.Fatalf("expected default UID to be: %v, got %v", "email", c.Schema.Mail)
	}

	// DN provided in config file
	if c.Schema.DN != ldapDefaults.DN {
		t.Fatalf("expected DisplayName to be: %v, got %v", ldapDefaults.DN, c.Schema.DN)
	}

	// positive tests for New
	_, err = New(map[string]interface{}{})
	if err != nil {
		t.Fatalf(err.Error())
	}
}
