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
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	datatx "github.com/cs3org/go-cs3apis/cs3/tx/v1beta1"
	txservice "github.com/cs3org/reva/v3/internal/grpc/services/datatx"
	revagateway "github.com/cs3org/reva/v3/internal/grpc/services/gateway"
	httpauth "github.com/cs3org/reva/v3/internal/http/interceptors/auth"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/strategy/bearer"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/strategy/header"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/tokenwriter/strategy/header"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/sciencemesh"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	txdriver "github.com/cs3org/reva/v3/pkg/datatx"
	txregistry "github.com/cs3org/reva/v3/pkg/datatx/manager/registry"
	txjson "github.com/cs3org/reva/v3/pkg/datatx/repository/json"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	_ "github.com/cs3org/reva/v3/pkg/share/cache/memory"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
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
	datatxServer   datatx.TxAPIServer
	webdavURL      string
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
	host, endpoint := "dest.example.org", "https://dest.example.org/dav"
	if p.webdavURL != "" {
		host, endpoint = p.webdavURL, p.webdavURL+"/dav"
	}
	return &ocmprovider.GetInfoByDomainResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		ProviderInfo: &ocmprovider.ProviderInfo{Services: []*ocmprovider.Service{{
			Host:     host,
			Endpoint: &ocmprovider.ServiceEndpoint{Type: &ocmprovider.ServiceType{Name: "webdav"}, Path: endpoint},
		}}},
	}, nil
}

func (p *mountedEmbeddedPeers) CreateTransfer(ctx context.Context, req *datatx.CreateTransferRequest, _ ...grpc.CallOption) (*datatx.CreateTransferResponse, error) {
	p.datatxCalls++
	if p.datatxServer != nil {
		return p.datatxServer.CreateTransfer(ctx, req)
	}
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

func mountEmbeddedGateway(t *testing.T, e *emitEnv, tokenManagers map[string]map[string]any) (mountedGateway, *mountedEmbeddedPeers) {
	t.Helper()
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
	client := mountedGateway{server: gw.(gateway.GatewayAPIServer)}
	stampGateway(client)
	return client, peers
}

type loopbackCopyDriver struct {
	client                   *http.Client
	starts, cancels, retries int
	done                     chan struct{}
	err                      error
}

const loopbackCopyID = "loopback-copy"

func (d *loopbackCopyDriver) CreateTransfer(_ context.Context, source, destination string) (*datatx.TxInfo, error) {
	d.starts++
	if d.starts != 1 {
		return nil, errors.New("unexpected second copy")
	}
	go func() {
		defer close(d.done)
		d.err = d.copy(source, destination)
	}()
	return &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: loopbackCopyID}, Status: datatx.Status_STATUS_TRANSFER_NEW}, nil
}

func (d *loopbackCopyDriver) copy(source, destination string) error {
	resp, err := d.client.Get(source)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("copy source returned %s", resp.Status)
	}
	target, err := url.Parse(destination)
	if err != nil {
		return err
	}
	target.Path, target.RawQuery = target.Query().Get("name"), ""
	req, err := http.NewRequest(http.MethodPut, target.String(), resp.Body)
	if err != nil {
		return err
	}
	out, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer out.Body.Close()
	if out.StatusCode != http.StatusCreated {
		return fmt.Errorf("copy destination returned %s", out.Status)
	}
	return nil
}

func (d *loopbackCopyDriver) GetTransferStatus(_ context.Context, id string) (*datatx.TxInfo, error) {
	if id != loopbackCopyID || d.starts == 0 {
		return nil, errors.New("unknown copy")
	}
	info := &datatx.TxInfo{Id: &datatx.TxId{OpaqueId: id}, Status: datatx.Status_STATUS_TRANSFER_IN_PROGRESS}
	select {
	case <-d.done:
		info.Status = datatx.Status_STATUS_TRANSFER_COMPLETE
		if d.err != nil {
			info.Status = datatx.Status_STATUS_TRANSFER_FAILED
			return info, d.err
		}
	default:
	}
	return info, nil
}

func (d *loopbackCopyDriver) CancelTransfer(context.Context, string) (*datatx.TxInfo, error) {
	d.cancels++
	return &datatx.TxInfo{}, errors.New("unexpected copy cancellation")
}

func (d *loopbackCopyDriver) RetryTransfer(context.Context, string) (*datatx.TxInfo, error) {
	d.retries++
	return &datatx.TxInfo{}, errors.New("unexpected copy retry")
}

func TestMountedDirectedDeclineRefusesLiveDATATX(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) { checkMountedLiveDATATX(t, newEmitEnv(t)) })
	t.Run("mysql", func(t *testing.T) {
		raw := os.Getenv("REVA_TEST_MYSQL")
		if raw == "" {
			t.Skip("disposable MySQL fixture is unavailable")
		}
		auth, addr, ok := strings.Cut(raw, "@")
		if !ok {
			t.Fatal("invalid REVA_TEST_MYSQL target")
		}
		dsn, err := mysql.ParseDSN(auth + "@tcp(" + addr + ")/")
		if err != nil {
			t.Fatal("invalid MySQL DSN")
		}
		host, portText, err := net.SplitHostPort(dsn.Addr)
		if err != nil {
			t.Fatal("invalid MySQL host and port")
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			t.Fatal("invalid MySQL port")
		}
		admin, err := sql.Open("mysql", dsn.FormatDSN())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = admin.Close() })
		name := "ocm_datatx_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := admin.Exec("CREATE DATABASE " + name + " CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP DATABASE " + name); err != nil {
				t.Errorf("drop disposable schema: %v", err)
			}
		})
		checkMountedLiveDATATX(t, newEmitEnvWithDatabase(t, map[string]any{
			"db_engine": "mysql", "db_host": host, "db_port": port, "db_name": name,
			"db_username": dsn.User, "db_password": dsn.Passwd,
		}))
	})
}

func checkMountedLiveDATATX(t *testing.T, e *emitEnv) {
	t.Helper()
	const payload = "live DATATX copy"
	destFile := filepath.Join(t.TempDir(), "doc.txt")
	entered, release := make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /source", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	})
	mux.HandleFunc("PUT /dav/transfers/doc.txt", func(w http.ResponseWriter, req *http.Request) {
		data, err := io.ReadAll(req.Body)
		if err != nil || string(data) != payload {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		close(entered)
		select {
		case <-release:
		case <-req.Context().Done():
			return
		}
		if err := os.WriteFile(destFile, data, 0600); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	peer := httptest.NewServer(mux)
	t.Cleanup(peer.Close)
	client := peer.Client()
	client.Timeout = 20 * time.Second
	driver := &loopbackCopyDriver{client: client, done: make(chan struct{})}
	wait := func(done <-chan struct{}) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("copy did not reach the expected data step")
		}
	}
	t.Cleanup(func() {
		finish()
		if driver.starts > 0 {
			wait(driver.done)
		}
	})

	const driverName = "directed-decline-loopback"
	txregistry.Register(driverName, func(context.Context, map[string]any) (txdriver.Manager, error) { return driver, nil })
	t.Cleanup(func() { delete(txregistry.NewFuncs, driverName) })
	repoConfig := map[string]any{"file": filepath.Join(t.TempDir(), "transfers.json")}
	tx, err := txservice.New(e.recip, map[string]any{
		"txdriver": driverName, "storagedriver": "json",
		"storagedrivers": map[string]map[string]any{"json": repoConfig},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Close() })
	txAPI := tx.(datatx.TxAPIServer)
	gw, peers := mountEmbeddedGateway(t, e, map[string]map[string]any{"jwt": {"secret": "datatx-route-test"}})
	peers.datatxServer, peers.webdavURL = txAPI, peer.URL
	const actorToken = "test-copy-token"
	e.recip = appctx.ContextSetToken(e.recip, actorToken)
	recipient := appctx.ContextMustGetUser(e.recip)
	recipient.Id.Type, recipient.Id.TenantId = userpb.UserType_USER_TYPE_PRIMARY, "test-tenant"
	sibling := e.received(t, "legacy-first", false, ocm.AccessType_ACCESS_TYPE_REMOTE)
	legacy := proto.CloneOf(sibling)
	legacy.Id, legacy.RemoteShareId = nil, "legacy-live"
	legacy.Protocols[0].GetWebdavOptions().Uri = peer.URL + "/source"
	legacy.Protocols[0].GetWebdavOptions().AccessTypes = []ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE, ocm.AccessType_ACCESS_TYPE_DATATX}
	legacy, err = e.repo.StoreReceivedShare(e.recip, legacy)
	if err != nil {
		t.Fatal(err)
	}
	update := func(state ocm.ShareState, want rpc.Code) {
		t.Helper()
		res, err := gw.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
			Share: &ocm.ReceivedShare{Id: legacy.Id, State: state}, UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
		})
		if err != nil || res.GetStatus().GetCode() != want {
			t.Fatalf("update to %v: status %v, error %v, want %v", state, res.GetStatus(), err, want)
		}
	}
	read := func(id *ocm.ShareId) *ocm.ReceivedShare {
		t.Helper()
		rs, err := e.repo.GetReceivedShare(e.recip, recipient, &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: id}})
		if err != nil {
			t.Fatal(err)
		}
		return rs
	}
	checkJob := func(want datatx.Status) {
		t.Helper()
		repo, err := txjson.New(e.recip, repoConfig)
		if err != nil {
			t.Fatal(err)
		}
		correlation, err := repo.GetTransfer(loopbackCopyID)
		if err != nil {
			t.Fatal(err)
		}
		if correlation.TxID != loopbackCopyID || correlation.ShareID != legacy.Id.OpaqueId ||
			!proto.Equal(correlation.UserID, recipient.Id) || correlation.SrcTargetURI != peer.URL+"/source" ||
			correlation.DestTargetURI != "http://"+actorToken+"@"+strings.TrimPrefix(peer.URL, "http://")+"?name=/dav/transfers/doc.txt" {
			t.Fatal("persisted job correlation differs from the scheduled share and actor")
		}
		response, err := txAPI.GetTransferStatus(e.recip, &datatx.GetTransferStatusRequest{TxId: &datatx.TxId{OpaqueId: loopbackCopyID}})
		info := response.GetTxInfo()
		if err != nil || response.GetStatus().GetCode() != rpc.Code_CODE_OK ||
			info.GetId().GetOpaqueId() != loopbackCopyID || !proto.Equal(info.GetShareId(), legacy.Id) || info.GetStatus() != want {
			t.Fatalf("copy status %v, error %v, want %s for the original attempt", response, err, want)
		}
		if peers.datatxCalls != 1 || driver.starts != 1 || driver.cancels != 0 || driver.retries != 0 {
			t.Fatalf("unexpected scheduling actions: dispatch=%d start=%d cancel=%d retry=%d", peers.datatxCalls, driver.starts, driver.cancels, driver.retries)
		}
	}

	siblingBefore := read(sibling.Id)
	update(ocm.ShareState_SHARE_STATE_ACCEPTED, rpc.Code_CODE_OK)
	wait(entered)
	actions := e.repo.(share.DirectedShareActions)
	refuse := func(state ocm.ShareState) {
		t.Helper()
		before := read(legacy.Id)
		if before.State != state {
			t.Fatalf("durable state %s, want %s", before.State, state)
		}
		checkJob(datatx.Status_STATUS_TRANSFER_IN_PROGRESS)
		updates, err := actions.RejectReceivedOCMSharesFromSender(e.recip, recipient.Id, legacy.Creator)
		var conflict errtypes.Conflict
		if updates != nil || !errors.Is(err, share.ErrDataTransferStateUnavailable) || !errors.As(err, &conflict) {
			t.Fatalf("directed action returned %d receipts and %v", len(updates), err)
		}
		if !proto.Equal(before, read(legacy.Id)) || !proto.Equal(siblingBefore, read(sibling.Id)) {
			t.Fatal("refused action changed a durable projection")
		}
		if _, err := os.Stat(destFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("copy destination before release: %v", err)
		}
		checkJob(datatx.Status_STATUS_TRANSFER_IN_PROGRESS)
		requireNotices(t, e.peer)
	}
	refuse(ocm.ShareState_SHARE_STATE_ACCEPTED)
	e.svc.conf.DisableDataTxAcceptance = true
	refuse(ocm.ShareState_SHARE_STATE_ACCEPTED)
	update(ocm.ShareState_SHARE_STATE_PENDING, rpc.Code_CODE_OK)
	update(ocm.ShareState_SHARE_STATE_ACCEPTED, rpc.Code_CODE_UNIMPLEMENTED)
	refuse(ocm.ShareState_SHARE_STATE_PENDING)
	e.svc.conf.DisableDataTxAcceptance = false
	refuse(ocm.ShareState_SHARE_STATE_PENDING)

	finish()
	wait(driver.done)
	checkJob(datatx.Status_STATUS_TRANSFER_COMPLETE)
	if data, err := os.ReadFile(destFile); err != nil || string(data) != payload {
		t.Fatalf("destination contains %q, error %v", data, err)
	}
}

func TestMountedEmbeddedShareProcessing(t *testing.T) {
	e := newEmitEnv(t)
	jwtConfig := map[string]any{"secret": "embedded-route-test"}
	tokenManagers := map[string]map[string]any{"jwt": jwtConfig}
	gw, peers := mountEmbeddedGateway(t, e, tokenManagers)
	update := func(id *ocm.ShareId, state ocm.ShareState, destination string, want rpc.Code) {
		t.Helper()
		res, err := gw.UpdateReceivedOCMShare(e.recip, &ocm.UpdateReceivedOCMShareRequest{
			Share:      &ocm.ReceivedShare{Id: id, State: state, Destination: destination},
			UpdateMask: &field_mask.FieldMask{Paths: []string{"state"}},
		})
		if err != nil || res.GetStatus() == nil || res.GetStatus().GetCode() != want {
			t.Fatalf("update to %v: status %v, error %v, want %v", state, res.GetStatus(), err, want)
		}
	}

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
		update(rs.Id, ocm.ShareState_SHARE_STATE_PENDING, "", rpc.Code_CODE_ABORTED)
		update(rs.Id, ocm.ShareState_SHARE_STATE_ACCEPTED, "/dest", rpc.Code_CODE_FAILED_PRECONDITION)
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
