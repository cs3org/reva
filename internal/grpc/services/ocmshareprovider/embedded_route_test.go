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

package ocmshareprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	revagateway "github.com/cs3org/reva/v3/internal/grpc/services/gateway"
	httpauth "github.com/cs3org/reva/v3/internal/http/interceptors/auth"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/strategy/bearer"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/strategy/header"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/tokenwriter/strategy/header"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/sciencemesh"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	_ "github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
)

type mountedGateway struct {
	gateway.GatewayAPIClient
	server gateway.GatewayAPIServer
}

func (g mountedGateway) UpdateReceivedOCMShare(ctx context.Context, req *ocm.UpdateReceivedOCMShareRequest, _ ...grpc.CallOption) (*ocm.UpdateReceivedOCMShareResponse, error) {
	return g.server.UpdateReceivedOCMShare(ctx, req)
}

type mountedEmbeddedPeers struct {
	ocm.OcmAPIClient
	ocmprovider.ProviderAPIClient
	datatx.TxAPIClient
	provider       *service
	datatxCalls    int
	datatxResponse *datatx.CreateTransferResponse
	updateCalls    int
	lookupOverride bool
	lookupResult   *ocm.GetReceivedOCMShareResponse
	lookupErr      error
}

func (r *gatewayResolver) OCMShareProvider(context.Context) (ocm.OcmAPIClient, error) {
	return r.embedded, nil
}

func (r *gatewayResolver) OCMProviderAuthorizer(context.Context) (ocmprovider.ProviderAPIClient, error) {
	return r.embedded, nil
}

func (r *gatewayResolver) DataTx(context.Context) (datatx.TxAPIClient, error) {
	return r.embedded, nil
}

func (p *mountedEmbeddedPeers) GetReceivedOCMShare(ctx context.Context, req *ocm.GetReceivedOCMShareRequest, _ ...grpc.CallOption) (*ocm.GetReceivedOCMShareResponse, error) {
	if p.lookupOverride {
		return p.lookupResult, p.lookupErr
	}
	return p.provider.GetReceivedOCMShare(ctx, req)
}

func (p *mountedEmbeddedPeers) UpdateReceivedOCMShare(ctx context.Context, req *ocm.UpdateReceivedOCMShareRequest, _ ...grpc.CallOption) (*ocm.UpdateReceivedOCMShareResponse, error) {
	p.updateCalls++
	return p.provider.UpdateReceivedOCMShare(ctx, req)
}

func (p *mountedEmbeddedPeers) GetInfoByDomain(context.Context, *ocmprovider.GetInfoByDomainRequest, ...grpc.CallOption) (*ocmprovider.GetInfoByDomainResponse, error) {
	return &ocmprovider.GetInfoByDomainResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		ProviderInfo: &ocmprovider.ProviderInfo{Services: []*ocmprovider.Service{{
			Host:     "dest.example.org",
			Endpoint: &ocmprovider.ServiceEndpoint{Type: &ocmprovider.ServiceType{Name: "webdav"}, Path: "https://dest.example.org/dav"},
		}}},
	}, nil
}

func (p *mountedEmbeddedPeers) CreateTransfer(context.Context, *datatx.CreateTransferRequest, ...grpc.CallOption) (*datatx.CreateTransferResponse, error) {
	p.datatxCalls++
	return p.datatxResponse, nil
}

type unacknowledgedTransferStart struct {
	share.Repository
}

func (r unacknowledgedTransferStart) StartReceivedShareTransfer(ctx context.Context, user *userpb.User, received *ocm.ReceivedShare, mask *field_mask.FieldMask) (*share.TransferStart, error) {
	receipt, err := r.Repository.StartReceivedShareTransfer(ctx, user, received, mask)
	if err != nil {
		return nil, err
	}
	return receipt, errCommitAcknowledgmentLost
}

func TestMountedEmbeddedShareProcessing(t *testing.T) {
	e := newEmitEnv(t)
	jwtConfig := map[string]any{"secret": "embedded-route-test"}
	tokenManagers := map[string]map[string]any{"jwt": jwtConfig}
	gw, err := revagateway.New(e.recip, map[string]any{
		"data_transfers_folder": "/transfers",
		"token_managers":        tokenManagers,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	peers := &mountedEmbeddedPeers{provider: e.svc}
	testGateways.embedded = peers
	t.Cleanup(func() { testGateways.embedded = nil })
	stampGateway(mountedGateway{server: gw.(gateway.GatewayAPIServer)})

	mesh, err := sciencemesh.New(e.recip, map[string]any{
		"gatewaysvc": "gateway", "mesh_directory_url": "https://directory.example.org",
		"provider_domain": "cernbox.example.org", "signing_key": writeKeyFile(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mesh.Close() })
	auth, err := httpauth.New(map[string]any{
		"credential_chain": []string{"bearer"},
		"token_managers":   tokenManagers,
	})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := jwt.New(jwtConfig)
	if err != nil {
		t.Fatal(err)
	}
	scopes, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	token, err := tokens.MintToken(e.recip, appctx.ContextMustGetUser(e.recip), scopes)
	if err != nil {
		t.Fatal(err)
	}
	mux := router.New()
	mesh.Routes(mux)
	handler := auth(mux)
	serve := func(req *http.Request) int {
		out := httptest.NewRecorder()
		route, ok := mux.Match(req)
		if !ok {
			t.Fatalf("route not mounted: %s", req.URL.Path)
		}
		handler.ServeHTTP(out, req.WithContext(router.WithRoute(req.Context(), route)))
		return out.Code
	}
	const route = "/sciencemesh/process-embedded-share"
	if code := serve(httptest.NewRequest(http.MethodPost, route, nil)); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request returned HTTP %d, want 401", code)
	}
	process := func(id *ocm.ShareId, destination string) int {
		q := url.Values{"share_id": {id.OpaqueId}, "destination": {destination}, "process": {"true"}}
		req := httptest.NewRequest(http.MethodPost, route+"?"+q.Encode(), nil)
		req.Header.Set(appctx.TokenHeader, token)
		return serve(req)
	}
	requireState := func(t *testing.T, id *ocm.ShareId, want ocm.ShareState) {
		t.Helper()
		current, err := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: id}})
		if err != nil {
			t.Fatal(err)
		}
		if current.State != want {
			t.Fatalf("stored state %s, want %s", current.State, want)
		}
	}

	for _, tc := range []struct {
		name     string
		override bool
		result   *ocm.GetReceivedOCMShareResponse
		err      error
		want     int
	}{
		{name: "stale reference", want: http.StatusNotFound},
		{name: "lookup transport error", override: true, err: errors.New("lookup unavailable"), want: http.StatusInternalServerError},
		{name: "lookup missing response", override: true, want: http.StatusInternalServerError},
		{name: "lookup missing status", override: true, result: &ocm.GetReceivedOCMShareResponse{}, want: http.StatusInternalServerError},
		{name: "lookup missing share", override: true, result: &ocm.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rs := e.received(t, tc.name, true)
			id := rs.Id
			if !tc.override {
				id = &ocm.ShareId{OpaqueId: "999999"}
			}
			peers.lookupOverride, peers.lookupResult, peers.lookupErr = tc.override, tc.result, tc.err
			t.Cleanup(func() { peers.lookupOverride, peers.lookupResult, peers.lookupErr = false, nil, nil })
			if code := process(id, "/dest"); code != tc.want {
				t.Errorf("HTTP %d, want %d", code, tc.want)
			}
			requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING)
			if peers.updateCalls != 0 || len(e.xfer.callbacks) != 0 || peers.datatxCalls != 0 ||
				len(e.svc.notifier.(*syncNotifier).results) != 0 {
				t.Fatal("failed lookup reached update, scheduling, or notification")
			}
			requireNotices(t, e.peer)
		})
	}

	t.Run("missing destination", func(t *testing.T) {
		rs := e.received(t, "missing-destination", true)
		if code := process(rs.Id, ""); code != http.StatusBadRequest {
			t.Fatalf("HTTP %d, want 400", code)
		}
		requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING)
		if len(e.xfer.callbacks) != 0 || peers.datatxCalls != 0 {
			t.Fatal("transfer started without a destination")
		}
		requireNotices(t, e.peer)
	})

	t.Run("uncertain commit", func(t *testing.T) {
		rs := e.received(t, "uncertain-commit", true)
		e.svc.repo = unacknowledgedTransferStart{Repository: e.repo}
		t.Cleanup(func() { e.svc.repo = e.repo })
		if code := process(rs.Id, "/dest"); code != http.StatusInternalServerError {
			t.Fatalf("HTTP %d, want 500", code)
		}
		requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_TRANSFERRING)
		if len(e.xfer.callbacks) != 0 || peers.datatxCalls != 0 {
			t.Fatal("transfer started without an acknowledged commit")
		}
		requireNotices(t, e.peer)
	})

	t.Run("mixed grant and refused repeat", func(t *testing.T) {
		e.svc.conf.DisableDataTxAcceptance = true
		t.Cleanup(func() { e.svc.conf.DisableDataTxAcceptance = false })
		rs := e.received(t, "mixed", true, ocm.AccessType_ACCESS_TYPE_DATATX)
		if code := process(rs.Id, "/dest"); code != http.StatusAccepted {
			t.Fatalf("HTTP %d, want 202 for scheduling", code)
		}
		requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_TRANSFERRING)
		if len(e.xfer.callbacks) != 1 || peers.datatxCalls != 0 {
			t.Fatalf("embedded starts %d, DATATX calls %d", len(e.xfer.callbacks), peers.datatxCalls)
		}
		requireNotices(t, e.peer)
		e.xfer.callback(0)(nil)
		requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED)
		requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
		if code := process(rs.Id, "/dest"); code != http.StatusBadRequest {
			t.Fatalf("repeat returned HTTP %d, want 400", code)
		}
		requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED)
		if len(e.xfer.callbacks) != 1 || peers.datatxCalls != 0 {
			t.Fatal("repeat started another transfer")
		}
		requireNotices(t, e.peer, ocmd.NotificationShareAccepted)
	})

	t.Run("DATATX disabled", func(t *testing.T) {
		e.svc.conf.DisableDataTxAcceptance = true
		t.Cleanup(func() { e.svc.conf.DisableDataTxAcceptance = false })
		t.Run("direct", func(t *testing.T) {
			rs := e.received(t, "datatx-disabled-direct", false, ocm.AccessType_ACCESS_TYPE_DATATX)
			res, err := e.svc.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
				Share:      &ocm.ReceivedShare{Id: rs.Id, State: ocm.ShareState_SHARE_STATE_ACCEPTED, Hidden: true},
				UpdateMask: &field_mask.FieldMask{Paths: []string{"state", "hidden"}},
			})
			if err != nil || res.GetStatus().GetCode() != rpc.Code_CODE_UNIMPLEMENTED {
				t.Errorf("status %v, error %v, want UNIMPLEMENTED", res.GetStatus(), err)
			}
			current, err := e.repo.GetReceivedShare(e.recip, appctx.ContextMustGetUser(e.recip), &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}})
			if err != nil {
				t.Fatal(err)
			}
			if current.GetState() != ocm.ShareState_SHARE_STATE_PENDING || current.GetHidden() {
				t.Fatalf("refusal changed state or hidden flag: %s, hidden=%t", current.GetState(), current.GetHidden())
			}
		})
		t.Run("mounted", func(t *testing.T) {
			rs := e.received(t, "datatx-disabled-mounted", false, ocm.AccessType_ACCESS_TYPE_DATATX)
			starts := peers.datatxCalls
			notices := len(e.svc.notifier.(*syncNotifier).results)
			peers.datatxResponse = &datatx.CreateTransferResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				TxInfo: &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: "unexpected-transfer"}, Status: datatx.Status_STATUS_TRANSFER_NEW},
			}
			t.Cleanup(func() { peers.datatxResponse = nil })
			if code := process(rs.Id, "/dest"); code != http.StatusNotImplemented {
				t.Errorf("HTTP %d, want 501", code)
			}
			if peers.datatxCalls != starts || len(e.svc.notifier.(*syncNotifier).results) != notices {
				t.Error("refused DATATX share reached scheduling or notification")
			}
			requireState(t, rs.Id, ocm.ShareState_SHARE_STATE_PENDING)
		})
	})

	for _, tc := range []struct {
		name     string
		response *datatx.CreateTransferResponse
		wantHTTP int
	}{
		{"scheduled", &datatx.CreateTransferResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			TxInfo: &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: "scheduled-transfer"}, Status: datatx.Status_STATUS_TRANSFER_NEW},
		}, http.StatusAccepted},
		{"refused", &datatx.CreateTransferResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL}}, http.StatusInternalServerError},
		{"missing transfer ID", &datatx.CreateTransferResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, http.StatusInternalServerError},
		{"failed job", &datatx.CreateTransferResponse{
			Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			TxInfo: &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: "failed-transfer"}, Status: datatx.Status_STATUS_TRANSFER_FAILED},
		}, http.StatusInternalServerError},
		{"missing status", &datatx.CreateTransferResponse{}, http.StatusInternalServerError},
		{"missing response", nil, http.StatusInternalServerError},
	} {
		t.Run("DATATX only "+tc.name, func(t *testing.T) {
			rs := e.received(t, "datatx-"+tc.name, false, ocm.AccessType_ACCESS_TYPE_DATATX)
			embeddedStarts := len(e.xfer.callbacks)
			datatxCalls := peers.datatxCalls
			notices := len(e.svc.notifier.(*syncNotifier).results)
			peers.datatxResponse = tc.response
			if code := process(rs.Id, "/dest"); code != tc.wantHTTP {
				t.Fatalf("HTTP %d, want %d", code, tc.wantHTTP)
			}
			if len(e.xfer.callbacks) != embeddedStarts || peers.datatxCalls != datatxCalls+1 {
				t.Fatalf("embedded starts %d, DATATX calls %d", len(e.xfer.callbacks)-embeddedStarts, peers.datatxCalls-datatxCalls)
			}
			if len(e.svc.notifier.(*syncNotifier).results) != notices {
				t.Fatal("legacy DATATX acceptance attempted a notification")
			}
		})
	}
}
