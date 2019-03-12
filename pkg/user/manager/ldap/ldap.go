package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/user"
	"github.com/cernbox/reva/pkg/user/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"gopkg.in/ldap.v2"
)

var logger = log.New("user-manager-ldap")

func init() {
	registry.Register("ldap", New)
}

type manager struct {
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
		logger.Error(context.Background(), errors.Wrap(err, "error decoding conf"))
		return nil, err
	}
	return c, nil
}

// New returns a user manager implementation that connects to a LDAP server to provide user metadata.
func New(m map[string]interface{}) (user.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	return &manager{
		hostname:     c.Hostname,
		port:         c.Port,
		baseDN:       c.BaseDN,
		filter:       c.Filter,
		bindUsername: c.BindUsername,
		bindPassword: c.BindPassword,
	}, nil
}

func (m *manager) GetUser(ctx context.Context, username string) (*user.User, error) {
	l, err := ldap.DialTLS("tcp", fmt.Sprintf("%s:%d", m.hostname, m.port), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return nil, err
	}
	defer l.Close()

	// First bind with a read only user
	err = l.Bind(m.bindUsername, m.bindPassword)
	if err != nil {
		return nil, err
	}

	// Search for the given clientID
	searchRequest := ldap.NewSearchRequest(
		m.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(m.filter, username),              // TODO this is screaming for errors if filter contains >1 %s
		[]string{"dn", "uid", "mail", "displayName"}, // TODO mapping
		nil,
	)

	sr, err := l.Search(searchRequest)
	if err != nil {
		return nil, err
	}

	if len(sr.Entries) != 1 {
		return nil, userNotFoundError(username)
	}

	for _, e := range sr.Entries {
		e.Print()
	}

	return &user.User{
		Username:    sr.Entries[0].GetAttributeValue("uid"),
		Groups:      []string{},
		Mail:        sr.Entries[0].GetAttributeValue("mail"),
		DisplayName: sr.Entries[0].GetAttributeValue("displayName"),
	}, nil
}

func (m *manager) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	return []string{}, nil
}

func (m *manager) IsInGroup(ctx context.Context, username, group string) (bool, error) {
	return false, nil
}

type userNotFoundError string

func (e userNotFoundError) Error() string { return string(e) }
