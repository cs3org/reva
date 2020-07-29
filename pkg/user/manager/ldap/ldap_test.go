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

package ldap

import (
	"testing"
)

type TestCasesStruc struct {
	name          string
	input         map[string]interface{}
}

var testCasesNegative = []TestCasesStruc{
	{"Negative",map[string]interface{}{"hostname": 42}},
	{"Negative", map[string]interface{}{"hostname": 2.6}},
	{"Negative",map[string]interface{}{"hostname": 40, "dn":"dn"}},
	{"Negative", map[string]interface{}{"user": 42,"hostname": 40}},
	{"Negative", map[string]interface{}{"port": "xyz"}},
	{"Negative", map[string]interface{}{"bind_username": 5678999}},
	{"Negative", map[string]interface{}{"bind_password":123456789}},
	{"Negative", map[string]interface{}{"base_dn": 99}},
	{"Negative", map[string]interface{}{"idp": 99}},
	{"Negative", map[string]interface{}{"userfilter": 99}},
	{"Negative", map[string]interface{}{"findfilter": 99}},
	{"Negative", map[string]interface{}{"groupfilter": 99}},
	}

var testCasesPositive = []TestCasesStruc{
	{"Positive", map[string]interface{}{"user": 42}},
	{"Positive", map[string]interface{}{}},
	{"Positive",map[string]interface{}{"hostname":""}},
	{"Positive",map[string]interface{}{"hostname": "host", "dn":"dn"}},
	{"Negative", map[string]interface{}{"port": 9090}},
	{"Negative", map[string]interface{}{"bind_username": "5678999"}},
	{"Negative", map[string]interface{}{"bind_password":"123456789"}},
	{"Negative", map[string]interface{}{"base_dn": "dn"}},
	{"Negative", map[string]interface{}{"idp": "idp"}},
	{"Negative", map[string]interface{}{"userfilter": "username"}},
	{"Negative", map[string]interface{}{"findfilter": "filter"}},
	{"Negative", map[string]interface{}{"groupfilter": "group"}},
}

func TestUserManager(t *testing.T) {
	for _,testCase := range testCasesNegative{
		t.Run(testCase.name, func(t *testing.T){
			_, err := New(testCase.input)
			if err == nil {
				t.Fatal("expected error but got none")
			}
		})
	}

	for _,testCase := range testCasesPositive{
		t.Run(testCase.name, func(t *testing.T){
			_, err := New(testCase.input)
			if err != nil {
				t.Fatalf(err.Error())
			}
		})
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
}
