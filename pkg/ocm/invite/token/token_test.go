package token

import (
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"sync"
	"testing"
)

func TestCreateToken(t *testing.T) {

	user := userpb.User{
		Id: &userpb.UserId{
			Idp:      "http://localhost:20080",
			OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
		},
		Username:     "",
		Mail:         "",
		MailVerified: false,
		DisplayName:  "",
		Groups:       nil,
		Opaque:       nil,
	}

	token, err := CreateToken("24h", user.GetId())
	if err != nil {
		t.Errorf("CreateToken() error = %v", err)
	}
	if token == nil {
		t.Errorf("CreateToken() got = %v", token)
	}
	if token.GetToken() == "" {
		t.Errorf("CreateToken() got = %v", token)
	}
	if token.GetUserId() != user.GetId() {
		t.Errorf("CreateToken() got = %v", token)
	}
}

func TestCreateTokenCollision(t *testing.T) {

	tokens := sync.Map{}

	user := userpb.User{
		Id: &userpb.UserId{
			Idp:      "http://localhost:20080",
			OpaqueId: "4c510ada-c86b-4815-8820-42cdf82c3d51",
		},
		Username:     "",
		Mail:         "",
		MailVerified: false,
		DisplayName:  "",
		Groups:       nil,
		Opaque:       nil,
	}

	for i := 0; i < 1000000; i++ {
		token, err := CreateToken("24h", user.GetId())
		if err != nil {
			t.Errorf("CreateToken() error = %v", err)
		}
		if token == nil {
			t.Errorf("CreateToken() token = %v", token)
		}

		_, ok := tokens.Load(token.GetToken())
		if ok == true {
			t.Errorf("CreateToken() there are ID collision  = %v", token)
		}

		tokens.Store(token.GetToken(), token)
	}
}
