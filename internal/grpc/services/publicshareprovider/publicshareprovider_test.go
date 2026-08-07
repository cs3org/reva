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

package publicshareprovider

import (
	"context"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/publicshare"
)

var (
	owner    = &userpb.User{Id: &userpb.UserId{Idp: "cern.ch", OpaqueId: "einstein"}}
	attacker = &userpb.User{Id: &userpb.UserId{Idp: "cern.ch", OpaqueId: "marie"}}
)

func TestIsManager(t *testing.T) {
	t.Parallel()

	id := func(idp, opaque string) *userpb.UserId {
		return &userpb.UserId{Idp: idp, OpaqueId: opaque}
	}

	tests := []struct {
		name  string
		user  *userpb.User
		share *link.PublicShare
		want  bool
	}{
		{
			name:  "creator may manage",
			user:  owner,
			share: &link.PublicShare{Creator: id("cern.ch", "einstein")},
			want:  true,
		},
		{
			name:  "resource owner may manage",
			user:  owner,
			share: &link.PublicShare{Creator: id("cern.ch", "curie"), Owner: id("cern.ch", "einstein")},
			want:  true,
		},
		{
			name: "sql driver drops the idp, opaque id still matches",
			user: owner,
			// what MakeUserID produces when reading a link back from the database
			share: &link.PublicShare{Creator: &userpb.UserId{OpaqueId: "einstein"}},
			want:  true,
		},
		{
			name:  "another user may not manage",
			user:  attacker,
			share: &link.PublicShare{Creator: id("cern.ch", "einstein"), Owner: id("cern.ch", "einstein")},
			want:  false,
		},
		{
			name:  "same name at another provider is a different person",
			user:  owner,
			share: &link.PublicShare{Creator: id("example.org", "einstein")},
			want:  false,
		},
		{
			name:  "link visitors carry no matching id",
			user:  &userpb.User{Id: &userpb.UserId{OpaqueId: "publiclink:42", Type: userpb.UserType_USER_TYPE_GUEST}},
			share: &link.PublicShare{Creator: id("cern.ch", "einstein")},
			want:  false,
		},
		{
			name:  "empty ids never match",
			user:  &userpb.User{Id: &userpb.UserId{}},
			share: &link.PublicShare{Creator: &userpb.UserId{}},
			want:  false,
		},
		{
			name:  "share without creator or owner",
			user:  owner,
			share: &link.PublicShare{},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isManager(tt.user, tt.share); got != tt.want {
				t.Errorf("isManager() = %v, want %v", got, tt.want)
			}
		})
	}
}

// stubManager serves a single share and records whether the write paths ran.
type stubManager struct {
	publicshare.Manager
	share   *link.PublicShare
	revoked bool
	updated bool
}

func (m *stubManager) GetPublicShare(ctx context.Context, u *userpb.User, ref *link.PublicShareReference, sign bool) (*link.PublicShare, error) {
	return m.share, nil
}

func (m *stubManager) RevokePublicShare(ctx context.Context, u *userpb.User, ref *link.PublicShareReference) error {
	m.revoked = true
	return nil
}

func (m *stubManager) UpdatePublicShare(ctx context.Context, u *userpb.User, req *link.UpdatePublicShareRequest, g *link.Grant) (*link.PublicShare, error) {
	m.updated = true
	return m.share, nil
}

func (m *stubManager) CreatePublicShare(ctx context.Context, u *userpb.User, md *provider.ResourceInfo, g *link.Grant, description string, internal bool, notifyUploads bool, notifyUploadsExtraRecipients string) (*link.PublicShare, error) {
	return m.share, nil
}

func byID(id string) *link.PublicShareReference {
	return &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{Id: &link.PublicShareId{OpaqueId: id}},
	}
}

func byToken(tkn string) *link.PublicShareReference {
	return &link.PublicShareReference{
		Spec: &link.PublicShareReference_Token{Token: tkn},
	}
}

// einstein's link, as the managers would return it
func einsteinsShare() *link.PublicShare {
	return &link.PublicShare{
		Id:      &link.PublicShareId{OpaqueId: "7"},
		Token:   "sometoken",
		Creator: &userpb.UserId{Idp: "cern.ch", OpaqueId: "einstein"},
		Owner:   &userpb.UserId{Idp: "cern.ch", OpaqueId: "einstein"},
	}
}

func TestPublicShareAccessControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		user *userpb.User
		ref  *link.PublicShareReference
		// whether the caller is expected to get through
		allowed bool
	}{
		{
			name:    "owner reads own link by id",
			user:    owner,
			ref:     byID("7"),
			allowed: true,
		},
		{
			name:    "stranger reads someone else's link by id",
			user:    attacker,
			ref:     byID("7"),
			allowed: false,
		},
		{
			name: "link visitor resolves the link by token",
			user: &userpb.User{Id: &userpb.UserId{OpaqueId: "publiclink:7", Type: userpb.UserType_USER_TYPE_GUEST}},
			ref:  byToken("sometoken"),
			// resolving by token has to keep working, that is how public links are served
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &service{sm: &stubManager{share: einsteinsShare()}}
			ctx := appctx.ContextSetUser(context.Background(), tt.user)

			res, err := svc.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: tt.ref})
			if err != nil {
				t.Fatal(err)
			}
			gotAllowed := res.Status.Code == rpc.Code_CODE_OK
			if gotAllowed != tt.allowed {
				t.Fatalf("GetPublicShare: allowed = %v (status %v), want %v", gotAllowed, res.Status.Code, tt.allowed)
			}
			if !tt.allowed && res.Share != nil {
				t.Error("a denied read must not return the share")
			}
		})
	}
}

func TestUpdateAndRemoveRequireOwnership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		user    *userpb.User
		ref     *link.PublicShareReference
		allowed bool
	}{
		{name: "owner updates and removes by id", user: owner, ref: byID("7"), allowed: true},
		{name: "stranger with the id", user: attacker, ref: byID("7"), allowed: false},
		{name: "stranger with the token", user: attacker, ref: byToken("sometoken"), allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := appctx.ContextSetUser(context.Background(), tt.user)

			sm := &stubManager{share: einsteinsShare()}
			svc := &service{sm: sm}
			updRes, err := svc.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: tt.ref})
			if err != nil {
				t.Fatal(err)
			}
			if got := updRes.Status.Code == rpc.Code_CODE_OK; got != tt.allowed {
				t.Errorf("UpdatePublicShare: allowed = %v (status %v), want %v", got, updRes.Status.Code, tt.allowed)
			}
			if sm.updated != tt.allowed {
				t.Errorf("UpdatePublicShare reached the manager = %v, want %v", sm.updated, tt.allowed)
			}

			sm = &stubManager{share: einsteinsShare()}
			svc = &service{sm: sm}
			remRes, err := svc.RemovePublicShare(ctx, &link.RemovePublicShareRequest{Ref: tt.ref})
			if err != nil {
				t.Fatal(err)
			}
			if got := remRes.Status.Code == rpc.Code_CODE_OK; got != tt.allowed {
				t.Errorf("RemovePublicShare: allowed = %v (status %v), want %v", got, remRes.Status.Code, tt.allowed)
			}
			if sm.revoked != tt.allowed {
				t.Errorf("RemovePublicShare reached the manager = %v, want %v", sm.revoked, tt.allowed)
			}
		})
	}
}

func TestAnonymousCallersAreRejected(t *testing.T) {
	t.Parallel()

	sm := &stubManager{share: einsteinsShare()}
	svc := &service{sm: sm}
	ctx := context.Background() // no user

	if res, _ := svc.GetPublicShare(ctx, &link.GetPublicShareRequest{Ref: byID("7")}); res.Status.Code == rpc.Code_CODE_OK {
		t.Error("GetPublicShare must not serve a request without a user")
	}
	if res, _ := svc.UpdatePublicShare(ctx, &link.UpdatePublicShareRequest{Ref: byID("7")}); res.Status.Code == rpc.Code_CODE_OK {
		t.Error("UpdatePublicShare must not serve a request without a user")
	}
	if res, _ := svc.RemovePublicShare(ctx, &link.RemovePublicShareRequest{Ref: byID("7")}); res.Status.Code == rpc.Code_CODE_OK {
		t.Error("RemovePublicShare must not serve a request without a user")
	}
	if sm.updated || sm.revoked {
		t.Error("an anonymous request reached the share manager")
	}
}
