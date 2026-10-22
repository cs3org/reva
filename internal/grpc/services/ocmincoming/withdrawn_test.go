// Copyright 2018-2024 CERN
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

package ocmincoming

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
)

func TestStaleCreateOfWithdrawnGrantIsRefused(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlmgr.NewOCMShareManager(ctx, map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "ocm.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{repo: repo}
	req := func(remote string) *ocmincoming.CreateOCMIncomingShareRequest {
		return &ocmincoming.CreateOCMIncomingShareRequest{
			Name:               "doc.txt",
			ResourceId:         remote,
			Owner:              &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
			Sender:             &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
			ShareWith:          &userpb.UserId{OpaqueId: "marie"},
			SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
			RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		}
	}
	if res, err := s.CreateOCMIncomingShare(ctx, req("42")); err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("create: %v %v", res, err)
	}
	if r, err := repo.WithdrawReceivedGrant(ctx, &share.GrantWithdrawal{
		Origin: "a.example.org", RemoteShareID: "42", ResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
	}); err != nil || !r.Changed {
		t.Fatalf("withdraw: %v %v", r, err)
	}
	res, err := s.CreateOCMIncomingShare(ctx, req("42"))
	if err != nil || res.Status.Code != rpc.Code_CODE_FAILED_PRECONDITION || res.Status.Message != share.WithdrawnShareMessage {
		t.Fatalf("stale create: %v %v", res, err)
	}
	if res, err := s.CreateOCMIncomingShare(ctx, req("43")); err != nil || res.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("fresh grant: %v %v", res, err)
	}
}

func TestUnrepresentableGrantIsInvalidArgument(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlmgr.NewOCMShareManager(ctx, map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "ocm.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{repo: repo}
	for _, remote := range []string{"", strings.Repeat("r", share.MaxGrantFieldLength+1)} {
		res, err := s.CreateOCMIncomingShare(ctx, &ocmincoming.CreateOCMIncomingShareRequest{
			Name:               "doc.txt",
			ResourceId:         remote,
			Owner:              &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
			Sender:             &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
			ShareWith:          &userpb.UserId{OpaqueId: "marie"},
			SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
			RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		})
		if err != nil || res.Status.Code != rpc.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("%d-byte remote id: %v %v", len(remote), res, err)
		}
	}
	shares, err := repo.ListReceivedShares(ctx, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}}, nil)
	if err != nil || len(shares) != 0 {
		t.Fatalf("stored %d shares: %v", len(shares), err)
	}
}

// countingRepo records every repository call made by the service.
type countingRepo struct {
	share.Repository
	calls int
}

func (r *countingRepo) StoreReceivedShare(ctx context.Context, rs *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	r.calls++
	return r.Repository.StoreReceivedShare(ctx, rs)
}

func TestMalformedSenderNeverReachesTheRepository(t *testing.T) {
	ctx := context.Background()
	inner, err := sqlmgr.NewOCMShareManager(ctx, map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "ocm.sqlite"),
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &countingRepo{Repository: inner}
	s := &service{repo: repo}
	for _, sender := range []*userpb.UserId{
		nil,
		{OpaqueId: "", Idp: "a.example.org"},
		{OpaqueId: "alice", Idp: ""},
		{OpaqueId: "alice", Idp: "evil.example@a.example.org"},
		{OpaqueId: "alice", Idp: "https://a.example.org"},
		{OpaqueId: "alice", Idp: "a.example.org:x8443"},
	} {
		res, err := s.CreateOCMIncomingShare(ctx, &ocmincoming.CreateOCMIncomingShareRequest{
			Name:               "doc.txt",
			ResourceId:         "42",
			Owner:              &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
			Sender:             sender,
			ShareWith:          &userpb.UserId{OpaqueId: "marie"},
			SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
			RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		})
		if err != nil || res.Status.Code != rpc.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("sender %v: %v %v", sender, res, err)
		}
	}
	if repo.calls != 0 {
		t.Fatalf("repository called %d times", repo.calls)
	}
}
