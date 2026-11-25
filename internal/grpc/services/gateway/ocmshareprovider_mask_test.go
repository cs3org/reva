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

package gateway

import (
	"context"
	"sync"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
)

type fakeOCMShares struct {
	ocm.OcmAPIClient
	mu            sync.Mutex
	current       *ocm.ReceivedShare
	updates       int
	failure       *rpc.Status
	missingStatus string
}

func (f *fakeOCMShares) GetReceivedOCMShare(context.Context, *ocm.GetReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.GetReceivedOCMShareResponse, error) {
	return &ocm.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: f.current}, nil
}

func (f *fakeOCMShares) UpdateReceivedOCMShare(context.Context, *ocm.UpdateReceivedOCMShareRequest, ...grpc.CallOption) (*ocm.UpdateReceivedOCMShareResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates++
	switch f.missingStatus {
	case "nil response":
		return nil, nil
	case "missing status":
		return &ocm.UpdateReceivedOCMShareResponse{}, nil
	}
	if f.failure != nil {
		return &ocm.UpdateReceivedOCMShareResponse{Status: f.failure}, nil
	}
	return &ocm.UpdateReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK, Message: "masked"}}, nil
}

type fakeOCMAuthorizer struct {
	ocmprovider.ProviderAPIClient
	lookups int
}

func (f *fakeOCMAuthorizer) GetInfoByDomain(context.Context, *ocmprovider.GetInfoByDomainRequest, ...grpc.CallOption) (*ocmprovider.GetInfoByDomainResponse, error) {
	f.lookups++
	return &ocmprovider.GetInfoByDomainResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		ProviderInfo: &ocmprovider.ProviderInfo{Services: []*ocmprovider.Service{{
			Host:     "dest.example.org",
			Endpoint: &ocmprovider.ServiceEndpoint{Type: &ocmprovider.ServiceType{Name: "webdav"}, Path: "https://dest.example.org/remote.php/webdav"},
		}}},
	}, nil
}

type fakeTransfers struct {
	datatx.TxAPIClient
	creates int
}

func (f *fakeTransfers) CreateTransfer(context.Context, *datatx.CreateTransferRequest, ...grpc.CallOption) (*datatx.CreateTransferResponse, error) {
	f.creates++
	return &datatx.CreateTransferResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		TxInfo: &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: "transfer"}, Status: datatx.Status_STATUS_TRANSFER_NEW},
	}, nil
}

func pendingTransferShare(state ocm.ShareState) *ocm.ReceivedShare {
	return &ocm.ReceivedShare{
		Id:    &ocm.ShareId{OpaqueId: "rs-1"},
		Name:  "data",
		State: state,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id:   &provider.Grantee_UserId{UserId: &userpb.UserId{OpaqueId: "bob", Idp: "dest.example.org"}},
		},
		Protocols: []*ocm.Protocol{{Term: &ocm.Protocol_WebdavOptions{WebdavOptions: &ocm.WebDAVProtocol{
			Uri:         "https://src.example.org/remote.php/dav/ocm/x",
			AccessTypes: []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_DATATX},
		}}}},
	}
}

// Only an explicit state mask accepting the share starts a transfer; a stale
// state in req.Share never resolves a destination or creates a transfer.
func TestUpdateReceivedOCMShareTransferNeedsMaskedAcceptance(t *testing.T) {
	ctx := appctx.ContextSetToken(appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "bob"}}), "tok")
	cases := []struct {
		name          string
		folder        string
		paths         []string
		requested     ocm.ShareState
		current       ocm.ShareState
		code          rpc.Code
		transfers     int
		refused       bool
		missingStatus string
	}{
		{name: "hidden only with stale accepted, unconfigured", paths: []string{"hidden"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_OK},
		{name: "hidden only with stale accepted, configured", folder: "/transfers", paths: []string{"hidden"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_OK},
		{name: "empty mask with stale accepted", folder: "/transfers", requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_OK},
		{name: "masked rejection", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_REJECTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_OK},
		{name: "masked acceptance", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_OK, transfers: 1},
		{name: "masked acceptance with nil response", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_INTERNAL, missingStatus: "nil response"},
		{name: "masked acceptance with missing status", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_INTERNAL, missingStatus: "missing status"},
		{name: "masked acceptance refused by provider", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_PERMISSION_DENIED, refused: true},
		{name: "masked acceptance outcome unknown", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_PENDING, code: rpc.Code_CODE_UNKNOWN, refused: true},
		{name: "masked acceptance of an accepted share", folder: "/transfers", paths: []string{"state"}, requested: ocm.ShareState_SHARE_STATE_ACCEPTED, current: ocm.ShareState_SHARE_STATE_ACCEPTED, code: rpc.Code_CODE_FAILED_PRECONDITION},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shares := &fakeOCMShares{current: pendingTransferShare(tc.current), missingStatus: tc.missingStatus}
			if tc.refused {
				shares.failure = &rpc.Status{Code: tc.code}
			}
			authz := &fakeOCMAuthorizer{}
			tx := &fakeTransfers{}
			stampOCMPeers(shares, authz, tx)
			s := &svc{c: &config{DataTransfersFolder: tc.folder}}
			req := &ocm.UpdateReceivedOCMShareRequest{
				Share: &ocm.ReceivedShare{Id: &ocm.ShareId{OpaqueId: "rs-1"}, State: tc.requested, Hidden: true},
			}
			if tc.paths != nil {
				req.UpdateMask = &field_mask.FieldMask{Paths: tc.paths}
			}
			res, _ := s.UpdateReceivedOCMShare(ctx, req)
			if res.GetStatus().GetCode() != tc.code || shares.updates != 1 {
				t.Fatalf("status %v updates %d", res.GetStatus(), shares.updates)
			}
			if tx.creates != tc.transfers {
				t.Fatalf("transfers %d, want %d", tx.creates, tc.transfers)
			}
			if tc.transfers == 0 && authz.lookups != 0 {
				t.Fatalf("destination resolved %d times", authz.lookups)
			}
			if tc.code == rpc.Code_CODE_OK && tc.transfers == 0 && res.GetStatus().GetMessage() != "masked" {
				t.Fatalf("not the provider's masked result: %v", res.GetStatus())
			}
		})
	}
}
