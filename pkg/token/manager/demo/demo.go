package demo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"

	"github.com/cernbox/reva/pkg/token/manager/registry"

	"github.com/cernbox/reva/pkg/token"
)

func init() {
	registry.Register("demo", New)
}

type manager struct {
	vault map[string]token.Claims
}

// New returns a new token manager.
func New(m map[string]interface{}) (token.Manager, error) {
	v := getVault()
	return &manager{vault: v}, nil
}

func (m *manager) ForgeToken(ctx context.Context, claims token.Claims) (string, error) {
	encoded, err := encode(claims)
	if err != nil {
		return "", err
	}
	return encoded, nil
}

func (m *manager) DismantleToken(ctx context.Context, token string) (token.Claims, error) {
	decoded, err := decode(token)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func getVault() map[string]token.Claims {
	return nil
}

// from https://stackoverflow.com/questions/28020070/golang-serialize-and-deserialize-back
// go binary encoder
func encode(m token.Claims) (string, error) {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(m)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// from https://stackoverflow.com/questions/28020070/golang-serialize-and-deserialize-back
// go binary decoder
func decode(str string) (token.Claims, error) {
	m := token.Claims{}
	by, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return nil, err
	}
	b := bytes.Buffer{}
	b.Write(by)
	d := gob.NewDecoder(&b)
	err = d.Decode(&m)
	if err != nil {
		return nil, err
	}
	return m, nil
}
