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

package admin

import (
	"context"
	"testing"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"google.golang.org/protobuf/proto"
)

func TestUserIDArg(t *testing.T) {
	full := &userpb.UserId{Idp: "https://auth.cern.ch", OpaqueId: "marie", Type: userpb.UserType_USER_TYPE_PRIMARY}
	for _, c := range []struct {
		in   string
		want *userpb.UserId
	}{
		{UserArg(full), full},
		{`{"opaque_id":"marie","idp":"cernbox"}`, &userpb.UserId{OpaqueId: "marie", Idp: "cernbox"}},
		{"marie", &userpb.UserId{OpaqueId: "marie"}},
		{"", nil},
	} {
		var u UserID
		if err := u.UnmarshalText([]byte(c.in)); err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if !proto.Equal(u.ID(), c.want) {
			t.Fatalf("%q: got %v, want %v", c.in, u.ID(), c.want)
		}
	}
	for _, in := range []string{`{"opaque_id":`, `{"idp":"cernbox"}`} {
		var u UserID
		if err := u.UnmarshalText([]byte(in)); err == nil {
			t.Fatalf("%q: expected an error", in)
		}
	}
	if UserArg(nil) != "" || UserArg(&userpb.UserId{Idp: "cernbox"}) != "" {
		t.Fatal("expected no argument for no user")
	}
}

func TestGranteeArg(t *testing.T) {
	group := &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_GROUP, Id: &provider.Grantee_GroupId{GroupId: &grouppb.GroupId{OpaqueId: "physics"}}}
	var g Grantee
	if err := g.UnmarshalText([]byte(GranteeArg(group))); err != nil || !proto.Equal(g.Get(), group) {
		t.Fatalf("got %v, %v", g.Get(), err)
	}
	if err := g.UnmarshalText([]byte("marie")); err != nil || g.Get().GetUserId().GetOpaqueId() != "marie" {
		t.Fatalf("got %v, %v", g.Get(), err)
	}
	if err := g.UnmarshalText([]byte(`{"type":"GRANTEE_TYPE_USER"}`)); err == nil {
		t.Fatal("expected an error for a grantee without an id")
	}
}

func TestUserIDInvocation(t *testing.T) {
	type args struct {
		Owner UserID `arg:"owner,required" help:"the owner"`
	}
	set := invoke.NewSet()
	var got *userpb.UserId
	set.Add("get", "").Handle(invoke.Typed(func(_ context.Context, a args) (invoke.Result, error) {
		got = a.Owner.ID()
		return nil, nil
	}))
	if typ := set.Invocations()[0].Args[0].Type; typ != "admin.UserID" {
		t.Fatalf("catalogue type %q", typ)
	}
	id := &userpb.UserId{Idp: "cernbox", OpaqueId: "marie"}
	if _, err := set.Invoke(context.Background(), "get", map[string]any{"owner": UserArg(id)}); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, id) {
		t.Fatalf("got %v", got)
	}
}

func TestCandidates(t *testing.T) {
	a := &userpb.UserId{OpaqueId: "a", Type: userpb.UserType_USER_TYPE_PRIMARY}
	b := &userpb.UserId{OpaqueId: "b"}
	got := Candidates(a, nil, &userpb.UserId{}, b, &userpb.UserId{OpaqueId: "a", Type: userpb.UserType_USER_TYPE_PRIMARY})
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("got %v", got)
	}
}
