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
	dbsql "database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmincoming "github.com/cs3org/go-cs3apis/cs3/ocm/incoming/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	jsonrepo "github.com/cs3org/reva/v3/pkg/ocm/share/repository/json"
	"github.com/cs3org/reva/v3/pkg/permissions"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	sqlmgr "github.com/cs3org/reva/v3/pkg/share/manager/sql"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

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
			ShareWith:          &userpb.UserId{OpaqueId: "marie", Idp: "local.example"},
			SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
			RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		})
		if err != nil || res.Status.Code != rpc.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("%d-byte remote id: %v %v", len(remote), res, err)
		}
	}
	shares, err := repo.ListReceivedShares(ctx, &userpb.User{Id: &userpb.UserId{OpaqueId: "marie", Idp: "local.example"}}, nil)
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
			ShareWith:          &userpb.UserId{OpaqueId: "marie", Idp: "local.example"},
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

	// A legal opaque ID keeps its @ and spaces; only the host is lowercased.
	res, err := s.CreateOCMIncomingShare(ctx, &ocmincoming.CreateOCMIncomingShareRequest{
		Name:               "doc.txt",
		ResourceId:         "42",
		Owner:              &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
		Sender:             &userpb.UserId{OpaqueId: "opaque@kept ", Idp: "A.Example.org:8443"},
		ShareWith:          &userpb.UserId{OpaqueId: "marie", Idp: "local.example"},
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
	})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK || repo.calls != 1 {
		t.Fatalf("valid sender: %v %v, %d calls", res, err, repo.calls)
	}
	if r, err := inner.WithdrawReceivedGrant(ctx, &share.GrantWithdrawal{
		Origin: "a.example.org:8443", RemoteShareID: "42", ResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
	}); err != nil || !r.Changed {
		t.Fatalf("withdraw by sender origin: %v %v", r, err)
	}
}

func TestIncomingRecipientAdmission(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ocm.sqlite")
	repo, err := sqlmgr.NewOCMShareManager(ctx, map[string]any{"db_engine": "sqlite", "db_name": path})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{repo: repo}
	sender := &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"}
	request := &ocmincoming.CreateOCMIncomingShareRequest{
		Name: "doc.txt", ResourceId: "42", Owner: sender, Sender: sender,
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		Protocols: []*ocm.Protocol{share.NewWebDAVProtocol("https://a.example.org/dav/42", "fixture",
			&ocm.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}, nil, nil)},
	}
	for _, incomplete := range []*userpb.UserId{{OpaqueId: "marie"}, {Idp: "local.example"}} {
		request.ShareWith = incomplete
		if res, err := s.CreateOCMIncomingShare(ctx, request); err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_INVALID_ARGUMENT || res.GetId() != "" {
			t.Fatalf("incomplete recipient: %v, %v", res, err)
		}
	}
	db, err := dbsql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var rows, protocols int
	err = db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM ocm_received_shares), (SELECT COUNT(*) FROM ocm_received_share_protocols)").Scan(&rows, &protocols)
	if err != nil || rows != 0 || protocols != 0 {
		t.Fatalf("invalid requests left %d rows and %d protocols: %v", rows, protocols, err)
	}

	request.ShareWith = &userpb.UserId{OpaqueId: "marie", Idp: "local.example"}
	res, err := s.CreateOCMIncomingShare(ctx, request)
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetId() == "" {
		t.Fatalf("complete recipient with empty tenant: %v, %v", res, err)
	}
	user := &userpb.User{Id: request.ShareWith}
	list, err := repo.ListReceivedShares(ctx, user, nil)
	if err != nil || len(list) != 1 || list[0].Id.OpaqueId != res.GetId() {
		t.Fatalf("exact recipient list: %v, %v", list, err)
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: list[0].Id}}
	update := &ocm.ReceivedShare{Id: list[0].Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED}
	mask := &fieldmaskpb.FieldMask{Paths: []string{"state"}}
	for _, id := range []*userpb.UserId{{OpaqueId: "marie", Idp: "other.example"}, {OpaqueId: "marie", Idp: "local.example", TenantId: "other"}} {
		other := &userpb.User{Id: id}
		if got, err := repo.ListReceivedShares(ctx, other, nil); err != nil || len(got) != 0 {
			t.Fatalf("different recipient list: %v, %v", got, err)
		}
		if got, err := repo.GetReceivedShare(ctx, other, ref); got != nil || !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("different recipient get: %v, %v", got, err)
		}
		if got, err := repo.UpdateReceivedShare(ctx, other, update, mask); got != nil || !errors.Is(err, share.ErrShareNotFound) {
			t.Fatalf("different recipient update: %v, %v", got, err)
		}
	}
	if got, err := repo.UpdateReceivedShare(ctx, user, update, mask); err != nil || !got.StateChanged() {
		t.Fatalf("exact recipient update: %v, %v", got, err)
	}
	if got, err := repo.GetReceivedShare(ctx, user, ref); err != nil || got.GetState() != update.State {
		t.Fatalf("persisted state: %v, %v", got, err)
	}

	jsonRepo, err := jsonrepo.New(ctx, map[string]any{"file": filepath.Join(t.TempDir(), "ocm.json")})
	if err != nil {
		t.Fatal(err)
	}
	request.ShareWith = &userpb.UserId{OpaqueId: "marie"}
	if res, err := (&service{repo: jsonRepo, conf: config{AllowVerifiedEmptyRecipientIdp: true}}).CreateOCMIncomingShare(ctx, request); err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetId() == "" {
		t.Fatalf("ordinary JSON admission: %v, %v", res, err)
	}
}

type recipientDirectory struct {
	revaservice.Clients
	userpb.UserAPIClient
	request  *userpb.GetUserByClaimRequest
	response *userpb.GetUserByClaimResponse
	err      error
	calls    int
}

var testRecipientDirectory = &recipientDirectory{}

func (d *recipientDirectory) UserProvider(context.Context) (userpb.UserAPIClient, error) {
	return d, nil
}

func (d *recipientDirectory) GetUserByClaim(ctx context.Context, req *userpb.GetUserByClaimRequest, _ ...grpc.CallOption) (*userpb.GetUserByClaimResponse, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("lookup deadline missing")
	}
	d.calls++
	d.request = req
	return d.response, d.err
}

func TestBlankSQLRecipientRequiresLocalProof(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ocm.sqlite")
	repo, err := sqlmgr.NewOCMShareManager(ctx, map[string]any{"db_engine": "sqlite", "db_name": path})
	if err != nil {
		t.Fatal(err)
	}
	s := &service{repo: repo}
	directory := testRecipientDirectory
	*directory = recipientDirectory{}
	revaservice.SetGlobal(directory)
	id := &userpb.UserId{OpaqueId: "private-local-recipient", Type: userpb.UserType_USER_TYPE_PRIMARY}
	sender := &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"}
	req := &ocmincoming.CreateOCMIncomingShareRequest{
		Name: "doc.txt", ResourceId: "42", Owner: sender, Sender: sender, ShareWith: id,
		SharedResourceType: ocm.SharedResourceType_SHARE_RESOURCE_TYPE_FILE,
		RecipientType:      ocm.RecipientType_RECIPIENT_TYPE_USER,
		Protocols: []*ocm.Protocol{share.NewWebDAVProtocol("https://a.example.org/dav/42", "fixture",
			&ocm.SharePermissions{Permissions: permissions.NewViewerRole().CS3ResourcePermissions()}, nil, nil)},
	}
	checkRefusal := func(want rpc.Code) {
		t.Helper()
		res, err := s.CreateOCMIncomingShare(ctx, req)
		if err != nil || res.GetStatus() == nil || res.GetStatus().GetCode() != want || res.GetId() != "" ||
			strings.Contains(res.GetStatus().GetMessage(), id.OpaqueId) {
			t.Fatalf("incoming refusal: %v, %v (want %v)", res, err, want)
		}
	}
	checkRefusal(rpc.Code_CODE_INVALID_ARGUMENT)
	s.conf.AllowVerifiedEmptyRecipientIdp = true
	req.ResourceId = ""
	checkRefusal(rpc.Code_CODE_INVALID_ARGUMENT)
	req.ResourceId, req.ShareWith = "42", &userpb.UserId{}
	checkRefusal(rpc.Code_CODE_INVALID_ARGUMENT)
	req.ShareWith = id
	if directory.calls != 0 {
		t.Fatalf("disabled or invalid request reached the directory %d times", directory.calls)
	}
	for _, tc := range []struct {
		directory rpc.Code
		incoming  rpc.Code
	}{
		{rpc.Code_CODE_NOT_FOUND, rpc.Code_CODE_FAILED_PRECONDITION},
		{rpc.Code_CODE_ABORTED, rpc.Code_CODE_FAILED_PRECONDITION},
		{rpc.Code_CODE_UNIMPLEMENTED, rpc.Code_CODE_FAILED_PRECONDITION},
		{rpc.Code_CODE_INTERNAL, rpc.Code_CODE_INTERNAL},
		{rpc.Code_CODE_UNAVAILABLE, rpc.Code_CODE_INTERNAL},
	} {
		directory.response = &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: tc.directory, Message: id.OpaqueId}}
		checkRefusal(tc.incoming)
	}
	directory.response, directory.err = nil, errors.New(id.OpaqueId)
	checkRefusal(rpc.Code_CODE_INTERNAL)
	directory.err = nil
	checkRefusal(rpc.Code_CODE_INTERNAL)
	for _, other := range []*userpb.UserId{
		nil,
		{OpaqueId: id.OpaqueId},
		{OpaqueId: id.OpaqueId, Idp: "other.example", Type: id.Type},
		{OpaqueId: id.OpaqueId, TenantId: "other", Type: id.Type},
		{OpaqueId: "different", Type: id.Type},
	} {
		directory.response = &userpb.GetUserByClaimResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: &userpb.User{Id: other}}
		checkRefusal(rpc.Code_CODE_FAILED_PRECONDITION)
	}
	db, err := dbsql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var rows, protocols int
	err = db.QueryRowContext(ctx, "SELECT (SELECT COUNT(*) FROM ocm_received_shares), (SELECT COUNT(*) FROM ocm_received_share_protocols)").Scan(&rows, &protocols)
	if err != nil || rows != 0 || protocols != 0 {
		t.Fatalf("refusals left %d rows and %d protocols: %v", rows, protocols, err)
	}
	directory.response.User.Id = id
	res, err := s.CreateOCMIncomingShare(ctx, req)
	if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_OK || res.GetId() == "" {
		t.Fatalf("verified recipient: %v, %v", res, err)
	}
	if q := directory.request; q.GetClaim() != "userid" || q.GetValue() != id.OpaqueId || !q.GetSkipFetchingUserGroups() {
		t.Fatalf("unexpected directory request: %v", q)
	}
	list, err := repo.ListReceivedShares(ctx, &userpb.User{Id: id}, nil)
	if err != nil || len(list) != 1 || list[0].GetId().GetOpaqueId() != res.GetId() || len(list[0].GetProtocols()) != 1 {
		t.Fatalf("stored share and protocol: %v, %v", list, err)
	}
}
