package main

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/user"
	"github.com/hashicorp/go-plugin"
	"github.com/mitchellh/mapstructure"
)

// Manager is a real implementation of Manager interface.
type Manager struct {
	users []*userpb.User
}

type config struct {
	Users string `mapstructure:"users"`
}

func (c *config) init() {
	if c.Users == "" {
		c.Users = "/etc/revad/users.json"
	}
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	c.init()
	return c, nil
}

// New initializes the manager struct based on the configurations.
func (m *Manager) New(ml map[string]interface{}) error {
	c, err := parseConfig(ml)
	if err != nil {
		return err
	}

	f, err := ioutil.ReadFile(c.Users)
	if err != nil {
		return err
	}

	users := []*userpb.User{}

	err = json.Unmarshal(f, &users)
	if err != nil {
		return err
	}

	m.users = users

	return nil
}

// GetUser returns the user based on the uid.
func (m *Manager) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	for _, u := range m.users {
		if (u.Id.GetOpaqueId() == uid.OpaqueId || u.Username == uid.OpaqueId) && (uid.Idp == "" || uid.Idp == u.Id.GetIdp()) {
			return u, nil
		}
	}
	return nil, nil
}

// GetUserByClaim returns user based on the claim
func (m *Manager) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	for _, u := range m.users {
		if userClaim, err := extractClaim(u, claim); err == nil && value == userClaim {
			return u, nil
		}
	}
	return nil, errtypes.NotFound(value)
}

func extractClaim(u *userpb.User, claim string) (string, error) {
	switch claim {
	case "mail":
		return u.Mail, nil
	case "username":
		return u.Username, nil
	case "uid":
		if u.Opaque != nil && u.Opaque.Map != nil {
			if uidObj, ok := u.Opaque.Map["uid"]; ok {
				if uidObj.Decoder == "plain" {
					return string(uidObj.Value), nil
				}
			}
		}
	}
	return "", errors.New("json: invalid field")
}

// TODO(jfd) search Opaque? compare sub?
func userContains(u *userpb.User, query string) bool {
	query = strings.ToLower(query)
	return strings.Contains(strings.ToLower(u.Username), query) || strings.Contains(strings.ToLower(u.DisplayName), query) ||
		strings.Contains(strings.ToLower(u.Mail), query) || strings.Contains(strings.ToLower(u.Id.OpaqueId), query)
}

// FindUsers returns the user based on the query
func (m *Manager) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	users := []*userpb.User{}
	for _, u := range m.users {
		if userContains(u, query) {
			users = append(users, u)
		}
	}
	return users, nil
}

// GetUserGroups returns the user groups
func (m *Manager) GetUserGroups(ctx context.Context, uid *userpb.UserId) ([]string, error) {
	user, err := m.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return user.Groups, nil
}

// Handshake hashicorp go-plugin handshake
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]plugin.Plugin{
			"userprovider": &user.ProviderPlugin{Impl: &Manager{}},
		},
	})
}
