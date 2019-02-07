package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cernbox/reva/pkg/auth"
	"github.com/cernbox/reva/pkg/auth/manager/registry"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/ldap.v2"
)

func init() {
	registry.Register("ldap", New)
}

type mgr struct {
	hostname     string
	port         int
	baseDN       string
	filter       string
	bindUsername string
	bindPassword string
}

type config struct {
	Hostname     string `mapstructure:"hostname"`
	Port         int    `mapstructure:"port"`
	BaseDN       string `mapstructure:"base_dn"`
	Filter       string `mapstructure:"filter"`
	BindUsername string `mapstructure:"bind_username"`
	BindPassword string `mapstructure:"bind_password"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

// New returns an auth manager implementation that connects to a LDAP server to validate the user.
func New(m map[string]interface{}) (auth.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &mgr{
		hostname:     c.Hostname,
		port:         c.Port,
		baseDN:       c.BaseDN,
		filter:       c.Filter,
		bindUsername: c.BindUsername,
		bindPassword: c.BindPassword,
	}, nil
}

func (am *mgr) Authenticate(ctx context.Context, clientID, clientSecret string) error {
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", am.hostname, am.port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(am.bindUsername, am.bindPassword)
	if err != nil {
		return err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		am.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(am.filter, clientID),
		[]string{"dn"},
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return err
	}

	if len(sr.Entries) != 1 {
		return userNotFoundError(clientID)
	}

	for _, e := range sr.Entries {
		e.Print()
	}

	userdn := sr.Entries[0].DN

	// Bind as the user to verify their password
	err = l.Bind(userdn, clientSecret)
	if err != nil {
		return err
	}

	return nil

}

type userNotFoundError string

func (e userNotFoundError) Error() string   { return string(e) }
func (e userNotFoundError) IsUserNotFound() {}
