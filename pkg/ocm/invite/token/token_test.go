// Copyright 2018-2021 CERN
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

package token

import (
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
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
