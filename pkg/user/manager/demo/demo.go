package demo

import (
	"context"

	"github.com/cernbox/reva/pkg/user"
)

type manager struct {
	catalog map[string]*user.User
}

func New(m map[string]interface{}) (user.Manager, error) {
	cat := getUsers()
	return &manager{catalog: cat}, nil
}

func (m *manager) GetUser(ctx context.Context, username string) (*user.User, error) {
	if user, ok := m.catalog[username]; ok {
		return user, nil
	}
	return nil, userNotFoundError(username)
}

func (m *manager) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	user, err := m.GetUser(ctx, username)
	if err != nil {
		return nil, err
	}
	return user.Groups, nil
}

func (m *manager) IsInGroup(ctx context.Context, username, group string) (bool, error) {
	user, err := m.GetUser(ctx, username)
	if err != nil {
		return false, err
	}

	for _, g := range user.Groups {
		if group == g {
			return true, nil
		}
	}
	return false, nil
}

type userNotFoundError string

func (e userNotFoundError) Error() string { return string(e) }

func getUsers() map[string]*user.User {
	return map[string]*user.User{
		"einstein": &user.User{
			Username:    "einstein",
			Groups:      []string{"sailing-lovers", "violin-haters"},
			Mail:        "einstein@example.org",
			DisplayName: "Albert Einstein",
		},
		"marie": &user.User{
			Username:    "marie",
			Groups:      []string{"radium-lovers", "polonium-lovers"},
			Mail:        "marie@example.org",
			DisplayName: "Marie Curie",
		},
		"richard": &user.User{
			Username:    "richard",
			Groups:      []string{"quantum-lovers", "philosophy-haters"},
			Mail:        "richard@example.org",
			DisplayName: "Richard Feynman",
		},
	}
}
