// Copyright 2018-2026 CERN
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

package invite

import (
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func assertBadRequest(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}
	if _, ok := err.(errtypes.BadRequest); !ok {
		t.Fatalf("expected a concrete errtypes.BadRequest, got %T: %v", err, err)
	}
	if err.Error() != "error: bad request: "+wantMsg {
		t.Fatalf("expected error %q, got %q", wantMsg, err.Error())
	}
}

func TestValidateUserID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		id      *userpb.UserId
		wantErr string
	}{
		{"nil id", nil, "missing field"},
		{"blank opaque id", &userpb.UserId{OpaqueId: ""}, "blank field"},
		{"whitespace opaque id", &userpb.UserId{OpaqueId: "  "}, "blank field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertBadRequest(t, ValidateUserID(tc.id, "field"), tc.wantErr)
		})
	}

	t.Run("valid id", func(t *testing.T) {
		// empty idp and unset user type stay valid: they are optional
		if err := ValidateUserID(&userpb.UserId{OpaqueId: "alice"}, "field"); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("field name is carried", func(t *testing.T) {
		err := ValidateUserID(nil, "initiator id")
		assertBadRequest(t, err, "missing initiator id")
	})
}

func TestValidateRemoteUser(t *testing.T) {
	t.Run("nil user", func(t *testing.T) {
		assertBadRequest(t, ValidateRemoteUser(nil), "missing remote user")
	})
	t.Run("user without id", func(t *testing.T) {
		assertBadRequest(t, ValidateRemoteUser(&userpb.User{}), "missing remote user id")
	})
	t.Run("user with blank id", func(t *testing.T) {
		assertBadRequest(t, ValidateRemoteUser(&userpb.User{Id: &userpb.UserId{OpaqueId: " "}}), "blank remote user id")
	})
	t.Run("valid user", func(t *testing.T) {
		if err := ValidateRemoteUser(&userpb.User{Id: &userpb.UserId{OpaqueId: "alice"}}); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}

func TestValidateInviteToken(t *testing.T) {
	t.Run("nil token", func(t *testing.T) {
		assertBadRequest(t, ValidateInviteToken(nil), "missing invite token")
	})
	t.Run("blank secret", func(t *testing.T) {
		assertBadRequest(t, ValidateInviteToken(&invitepb.InviteToken{Token: ""}), "blank invite token")
	})
	t.Run("whitespace secret", func(t *testing.T) {
		assertBadRequest(t, ValidateInviteToken(&invitepb.InviteToken{Token: "  "}), "blank invite token")
	})
	t.Run("token without user id", func(t *testing.T) {
		assertBadRequest(t, ValidateInviteToken(&invitepb.InviteToken{Token: "secret"}), "missing invite token user id")
	})
	t.Run("token with blank user id", func(t *testing.T) {
		assertBadRequest(t, ValidateInviteToken(&invitepb.InviteToken{Token: "secret", UserId: &userpb.UserId{OpaqueId: ""}}), "blank invite token user id")
	})
	t.Run("valid token without expiration", func(t *testing.T) {
		// the CS3 expiration field is optional and must not be required
		token := &invitepb.InviteToken{Token: "secret", UserId: &userpb.UserId{OpaqueId: "initiator"}}
		if err := ValidateInviteToken(token); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
	t.Run("valid token with expiration", func(t *testing.T) {
		token := &invitepb.InviteToken{
			Token:      "secret",
			UserId:     &userpb.UserId{OpaqueId: "initiator"},
			Expiration: &typespb.Timestamp{Seconds: 42},
		}
		if err := ValidateInviteToken(token); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}
