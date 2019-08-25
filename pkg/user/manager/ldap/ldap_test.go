package ldap

import (
	"strings"
	"testing"

	configreader "github.com/cs3org/reva/cmd/revad/config"
	"gotest.tools/assert"
)

var configs = map[string]string{
	"ad_schema": `
		[schema]
			displayName = "displayName"
			dn = "dn"
			uid = "objectSid"
		`,
	"no_schema": ``,
	"mail_uid_set": `
		[schema]
			mail = "myEmailAttribute"
			uid = "someObscureSchema"
	`,
}

func TestAttributesDefaults(t *testing.T) {
	ad_schema := mustLoadConfig("ad_schema")
	assert.Equal(t, ad_schema.Schema.UID, "objectSid")
	assert.Equal(t, ad_schema.Schema.Mail, "mail") // mail not provided in config defaults to tag "mail"

	// no ldap schema provided - use defaults
	no_schema := mustLoadConfig("no_schema")
	assert.Equal(t, no_schema.Schema.Mail, "mail")
	assert.Equal(t, no_schema.Schema.UID, "objectGUID")
	assert.Equal(t, no_schema.Schema.DisplayName, "displayName")
	assert.Equal(t, no_schema.Schema.DN, "dn")

	// attributes defined in the config file take precedence
	mail_uid_set := mustLoadConfig("mail_uid_set")
	assert.Equal(t, mail_uid_set.Schema.Mail, "myEmailAttribute")
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
