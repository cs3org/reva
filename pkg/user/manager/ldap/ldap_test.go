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

package ldap

import (
	"strings"
	"testing"

	configreader "github.com/cs3org/reva/cmd/revad/config"
	"gotest.tools/assert"
)

var configs = map[string]string{
	"adSchema": `
		[schema]
			mail = "email"
			uid = "objectSID"
			displayName = "displayName"
			dn = "dn"
		`,
	"noSchema": ``,
	"partialSet": `
		[schema]
			mail = "myEmailAttribute"
			uid = "someObscureSchema"
	`,
	"invalidAttribute": `
		[schema]
			invalid = "myEmailAttribute"
	`,
}

func TestInitFromSchema(t *testing.T) {
	config := mustLoadConfig("adSchema")
	assert.Equal(t, config.Schema.Mail, "email")
	assert.Equal(t, config.Schema.UID, "objectSID")
	assert.Equal(t, config.Schema.DisplayName, "displayName")
	assert.Equal(t, config.Schema.DN, "dn")
}

func TestNoSchema(t *testing.T) {
	config := mustLoadConfig("noSchema")
	assert.Equal(t, config.Schema.Mail, "mail")
	assert.Equal(t, config.Schema.UID, "objectGUID")
	assert.Equal(t, config.Schema.DisplayName, "displayName")
	assert.Equal(t, config.Schema.DN, "dn")
}

func TestPartialSchemaProvided(t *testing.T) {
	config := mustLoadConfig("partialSet")
	assert.Equal(t, config.Schema.Mail, "myEmailAttribute")
	assert.Equal(t, config.Schema.UID, "someObscureSchema")
	assert.Equal(t, config.Schema.DN, "dn")
}

func TestIgnoreInvalidAttribute(t *testing.T) {
	config := mustLoadConfig("invalidAttribute")
	assert.Equal(t, config.Schema.Mail, "mail")
	assert.Equal(t, config.Schema.UID, "objectGUID")
	assert.Equal(t, config.Schema.DN, "dn")
}

func mustLoadConfig(key string) *config {
	r := strings.NewReader(configs[key])
	config, err := configreader.Read(r)
	if err != nil {
		panic("error reading configuration")
	}

	c, err := parseConfig(config)
	if err != nil {
		panic("error while parsing configuration from file")
	}

	return c
}
