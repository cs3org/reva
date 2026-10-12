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

package cache

import (
	"testing"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func testIdentity() *Identity {
	return &Identity{
		User: &userpb.User{
			Id:       &userpb.UserId{OpaqueId: "einstein", Idp: "cernbox"},
			Username: "einstein",
			Groups:   []string{"physics"},
		},
		Scopes: map[string]*authpb.Scope{
			"user": {Role: authpb.Role_ROLE_OWNER},
		},
		Token: "token",
	}
}

func TestAcceptedCredentialIsAnsweredFromTheCache(t *testing.T) {
	c := New(Config{})
	key := Key("basic", "einstein", "relativity")

	if _, _, ok := c.Get(key); ok {
		t.Fatal("expected a miss on an unknown credential")
	}

	c.Set(key, testIdentity(), nil, time.Time{})

	id, err, ok := c.Get(key)
	if !ok || err != nil {
		t.Fatalf("expected a hit without an error, got ok=%v err=%v", ok, err)
	}
	if id.User.Username != "einstein" || id.Token != "token" {
		t.Fatalf("unexpected identity %+v", id)
	}
	if id.Scopes["user"].Role != authpb.Role_ROLE_OWNER {
		t.Fatalf("unexpected scopes %+v", id.Scopes)
	}
}

func TestRejectedCredentialIsAnsweredFromTheCache(t *testing.T) {
	c := New(Config{})
	key := Key("basic", "einstein", "wrong")

	c.Set(key, nil, errtypes.InvalidCredentials("wrong password"), time.Time{})

	id, err, ok := c.Get(key)
	if !ok {
		t.Fatal("expected a hit on a rejected credential")
	}
	if id != nil {
		t.Fatalf("expected no identity, got %+v", id)
	}
	if _, is := err.(errtypes.InvalidCredentials); !is {
		t.Fatalf("expected the rejection to survive, got %v", err)
	}
}

func TestDifferentCredentialsDoNotShareAnEntry(t *testing.T) {
	c := New(Config{})
	c.Set(Key("basic", "einstein", "relativity"), testIdentity(), nil, time.Time{})

	if _, _, ok := c.Get(Key("basic", "einstein", "wrong")); ok {
		t.Fatal("a wrong secret must not hit the entry of the right one")
	}
	if _, _, ok := c.Get(Key("basic", "marie", "relativity")); ok {
		t.Fatal("another user must not hit the entry of einstein")
	}
	// The parts are separated, so a shifted split is a different key.
	if Key("ab", "c") == Key("a", "bc") {
		t.Fatal("keys must not collide on a shifted split")
	}
}

func TestEntryExpiresAfterTheTTL(t *testing.T) {
	c := New(Config{TTL: 1})
	key := Key("apptoken")
	c.Set(key, testIdentity(), nil, time.Time{})

	if _, _, ok := c.Get(key); !ok {
		t.Fatal("expected a hit before the TTL")
	}

	time.Sleep(1100 * time.Millisecond)

	if _, _, ok := c.Get(key); ok {
		t.Fatal("expected a miss after the TTL")
	}
}

func TestEntryNeverOutlivesTheCredential(t *testing.T) {
	c := New(Config{TTL: 300})
	key := Key("apptoken")

	// An expiration sooner than the TTL wins.
	c.Set(key, testIdentity(), nil, time.Now().Add(time.Second))
	if _, _, ok := c.Get(key); !ok {
		t.Fatal("expected a hit before the expiration")
	}
	time.Sleep(1100 * time.Millisecond)
	if _, _, ok := c.Get(key); ok {
		t.Fatal("expected a miss after the expiration")
	}

	// A credential that already expired is not stored at all.
	c.Set(key, testIdentity(), nil, time.Now().Add(-time.Second))
	if _, _, ok := c.Get(key); ok {
		t.Fatal("an expired credential must not be cached")
	}
}

func TestCallersCannotCorruptACachedEntry(t *testing.T) {
	c := New(Config{})
	key := Key("basic", "einstein", "relativity")

	stored := testIdentity()
	c.Set(key, stored, nil, time.Time{})
	stored.User.Username = "mallory"
	stored.Scopes["user"].Role = authpb.Role_ROLE_VIEWER

	first, _, _ := c.Get(key)
	if first.User.Username != "einstein" || first.Scopes["user"].Role != authpb.Role_ROLE_OWNER {
		t.Fatalf("modifying the stored identity leaked into the cache: %+v", first)
	}

	first.User.Groups = append(first.User.Groups, "maths")
	first.Scopes["extra"] = &authpb.Scope{}

	second, _, _ := c.Get(key)
	if len(second.User.Groups) != 1 || len(second.Scopes) != 1 {
		t.Fatalf("modifying a returned identity leaked into the cache: %+v", second)
	}
}

func TestNegativeTTLDisablesTheCache(t *testing.T) {
	c := New(Config{TTL: -1})
	key := Key("basic", "einstein", "relativity")

	c.Set(key, testIdentity(), nil, time.Time{})
	if _, _, ok := c.Get(key); ok {
		t.Fatal("a disabled cache must never report a hit")
	}
}
