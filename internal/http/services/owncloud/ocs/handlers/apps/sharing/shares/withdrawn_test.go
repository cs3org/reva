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

package shares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ReneKroon/ttlcache/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmshare "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/service"
	"google.golang.org/grpc"
)

type ocsResolver struct {
	service.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *ocsResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var (
	ocsTestResolver     = &ocsResolver{}
	ocsTestResolverOnce sync.Once
)

func stampOCSGateway(gw gateway.GatewayAPIClient) {
	ocsTestResolverOnce.Do(func() { service.SetGlobal(ocsTestResolver) })
	ocsTestResolver.mu.Lock()
	ocsTestResolver.gw = gw
	ocsTestResolver.mu.Unlock()
}

type withdrawnGateway struct {
	gateway.GatewayAPIClient
	share  *ocmpb.ReceivedShare
	list   []*ocmpb.ReceivedShare
	update *rpc.Status
}

func (g *withdrawnGateway) GetReceivedOCMShare(context.Context, *ocmpb.GetReceivedOCMShareRequest, ...grpc.CallOption) (*ocmpb.GetReceivedOCMShareResponse, error) {
	return &ocmpb.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: g.share}, nil
}

func (g *withdrawnGateway) UpdateReceivedOCMShare(context.Context, *ocmpb.UpdateReceivedOCMShareRequest, ...grpc.CallOption) (*ocmpb.UpdateReceivedOCMShareResponse, error) {
	return &ocmpb.UpdateReceivedOCMShareResponse{Status: g.update}, nil
}

func (g *withdrawnGateway) ListReceivedOCMShares(context.Context, *ocmpb.ListReceivedOCMSharesRequest, ...grpc.CallOption) (*ocmpb.ListReceivedOCMSharesResponse, error) {
	return &ocmpb.ListReceivedOCMSharesResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Shares: g.list}, nil
}

func (g *withdrawnGateway) GetUser(context.Context, *userpb.GetUserRequest, ...grpc.CallOption) (*userpb.GetUserResponse, error) {
	return &userpb.GetUserResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
}

func ocsShare(id string, state ocmpb.ShareState) *ocmpb.ReceivedShare {
	return &ocmpb.ReceivedShare{
		Id:      &ocmpb.ShareId{OpaqueId: id},
		Name:    "doc.txt",
		State:   state,
		Hidden:  state == ocmpb.ShareState_SHARE_STATE_INVALID,
		Owner:   &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
		Creator: &userpb.UserId{OpaqueId: "alice", Idp: "a.example.org"},
		Grantee: &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{
			UserId: &userpb.UserId{OpaqueId: "marie"},
		}},
		Ctime: &typespb.Timestamp{Seconds: 1},
		Protocols: []*ocmpb.Protocol{ocmshare.NewWebDAVProtocol("https://a.example.org/dav/"+id, "secret",
			&ocmpb.SharePermissions{Permissions: &provider.ResourcePermissions{Stat: true}}, nil, nil)},
	}
}

func ocsAccept(t *testing.T, version string, update *rpc.Status) (*httptest.ResponseRecorder, response.Meta) {
	t.Helper()
	stampOCSGateway(&withdrawnGateway{share: ocsShare("7", ocmpb.ShareState_SHARE_STATE_INVALID), update: update})
	h := &Handler{}
	r := http.NewServeMux()
	r.Handle("POST /v"+version+".php/shares/pending/{id}", response.VersionCtx(version)(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h.updateReceivedFederatedShare(w, req, req.PathValue("id"), false)
	})))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v"+version+".php/shares/pending/7?format=json", nil))
	var body struct {
		OCS struct {
			Meta response.Meta `json:"meta"`
		} `json:"ocs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return rec, body.OCS.Meta
}

func TestAcceptWithdrawnOCMShareIsConflict(t *testing.T) {
	withdrawn := &rpc.Status{Code: rpc.Code_CODE_FAILED_PRECONDITION, Message: ocmshare.WithdrawnShareMessage}
	rec, meta := ocsAccept(t, "1", withdrawn)
	if rec.Code != http.StatusOK || meta.StatusCode != http.StatusConflict || meta.Status != "error" || meta.Message != ocmshare.WithdrawnShareMessage {
		t.Fatalf("v1: http %d meta %+v", rec.Code, meta)
	}
	rec, meta = ocsAccept(t, "2", withdrawn)
	if rec.Code != http.StatusConflict || meta.StatusCode != http.StatusConflict {
		t.Fatalf("v2: http %d meta %+v", rec.Code, meta)
	}
	rec, meta = ocsAccept(t, "2", &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND})
	if rec.Code != http.StatusNotFound || meta.StatusCode == http.StatusConflict {
		t.Fatalf("not found stays separate: http %d meta %+v", rec.Code, meta)
	}
	rec, meta = ocsAccept(t, "2", &rpc.Status{Code: rpc.Code_CODE_FAILED_PRECONDITION, Message: "transfer already in progress"})
	if rec.Code == http.StatusConflict || meta.StatusCode == http.StatusConflict {
		t.Fatalf("another precondition became a withdrawal conflict: http %d meta %+v", rec.Code, meta)
	}
}

func TestActiveOCSListsOmitWithdrawnShares(t *testing.T) {
	// Without owner fields the listing needs no remote user lookups.
	anonymous := func(s *ocmpb.ReceivedShare) *ocmpb.ReceivedShare {
		s.Owner, s.Creator = nil, nil
		return s
	}
	gw := &withdrawnGateway{list: []*ocmpb.ReceivedShare{
		anonymous(ocsShare("1", ocmpb.ShareState_SHARE_STATE_ACCEPTED)),
		anonymous(ocsShare("2", ocmpb.ShareState_SHARE_STATE_PENDING)),
		anonymous(ocsShare("3", ocmpb.ShareState_SHARE_STATE_INVALID)),
	}}
	h := &Handler{userIdentifierCache: ttlcache.NewCache()}
	for state, want := range map[ocmpb.ShareState]string{
		ocmpb.ShareState_SHARE_STATE_ACCEPTED: "1",
		ocmpb.ShareState_SHARE_STATE_PENDING:  "2",
	} {
		ctx := appctx.ContextSetUser(context.Background(), &userpb.User{Id: &userpb.UserId{OpaqueId: "marie"}})
		got, err := h.listReceivedFederatedShares(ctx, gw, state)
		if err != nil || len(got) != 1 || got[0].ID != want {
			t.Fatalf("state %v: %v %v", state, got, err)
		}
	}
}
