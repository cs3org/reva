package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cernbox/reva/pkg/auth"
	"gopkg.in/ldap.v2"
)

type mgr struct {
	hostname     string
	port         int
	baseDN       string
	filter       string
	bindUsername string
	bindPassword string
}

// New returns an auth manager implementation that connects to a LDAP server to validate the user.
func New(hostname string, port int, baseDN, filter, bindUsername, bindPassword string) auth.Manager {
	return &mgr{
		hostname:     hostname,
		port:         port,
		baseDN:       baseDN,
		filter:       filter,
		bindUsername: bindUsername,
		bindPassword: bindPassword,
	}
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
