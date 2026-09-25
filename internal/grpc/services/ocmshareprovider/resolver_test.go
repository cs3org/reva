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

package ocmshareprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/ocm/embedded"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/ocm/share/repository/registry"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	testOpener      = "https://sender.example/services/ocm/open"
	testWebDAVRoot  = "https://sender.example/webdav"
	testAppName     = "SynthApp"
	testQuotedName  = `Synth "Lab" & <tag>`
	testPaddedName  = "  SynthApp  "
	testDriverName  = "offer-gate-test"
	testNoRunDriver = "offer-gate-should-not-run"
	secretMarker    = "synthetic-secret-marker"
)

func TestMain(m *testing.M) {
	revaservice.SetGlobal(testGateways)
	prevRepo, repoOK := registry.NewFuncs[testDriverName]
	prevEmb, embOK := embedded.NewFuncs[testDriverName]
	registry.Register(testDriverName, func(context.Context, map[string]any) (share.Repository, error) {
		return &capturingRepo{}, nil
	})
	embedded.Register(testDriverName, func(context.Context, map[string]any) (embedded.Transferrer, error) {
		return noopTransferrer{}, nil
	})
	code := m.Run()
	restoreRepoDriver(testDriverName, prevRepo, repoOK)
	restoreEmbeddedDriver(testDriverName, prevEmb, embOK)
	os.Exit(code)
}

func restoreRepoDriver(name string, prev registry.NewFunc, ok bool) {
	if ok {
		registry.NewFuncs[name] = prev
		return
	}
	delete(registry.NewFuncs, name)
}

func restoreEmbeddedDriver(name string, prev embedded.NewFunc, ok bool) {
	if ok {
		embedded.NewFuncs[name] = prev
		return
	}
	delete(embedded.NewFuncs, name)
}

func withTestDrivers(t *testing.T, name string, repo registry.NewFunc, transfer embedded.NewFunc) {
	t.Helper()
	prevRepo, repoOK := registry.NewFuncs[name]
	prevEmb, embOK := embedded.NewFuncs[name]
	registry.Register(name, repo)
	embedded.Register(name, transfer)
	t.Cleanup(func() {
		restoreRepoDriver(name, prevRepo, repoOK)
		restoreEmbeddedDriver(name, prevEmb, embOK)
	})
}

type gatewayResolver struct {
	revaservice.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *gatewayResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gw, nil
}

var testGateways = &gatewayResolver{}

func stampGateway(gw gateway.GatewayAPIClient) {
	testGateways.mu.Lock()
	testGateways.gw = gw
	testGateways.mu.Unlock()
}

type statGateway struct {
	gateway.GatewayAPIClient
	info *providerpb.ResourceInfo
}

func (g *statGateway) Stat(
	context.Context,
	*providerpb.StatRequest,
	...grpc.CallOption,
) (*providerpb.StatResponse, error) {
	return &providerpb.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info:   g.info,
	}, nil
}

type capturingRepo struct {
	share.Repository
	mu     sync.Mutex
	nextID string
	stored *ocm.Share
	storeN int
	idN    int
}

func (r *capturingRepo) GetShare(context.Context, *userpb.User, *ocm.ShareReference) (*ocm.Share, error) {
	return nil, share.ErrShareNotFound
}

func (r *capturingRepo) GenerateID(context.Context) (*ocm.ShareId, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idN++
	id := r.nextID
	if id == "" {
		id = "share-opaque"
	}
	return &ocm.ShareId{OpaqueId: id}, nil
}

func (r *capturingRepo) StoreShare(_ context.Context, s *ocm.Share) (*ocm.Share, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeN++
	r.stored = s
	return s, nil
}

type noopTransferrer struct{}

func (noopTransferrer) Process(context.Context, string, string, func(error)) error {
	return nil
}

type remotePeer struct {
	mu     sync.Mutex
	disco  []byte
	status int
	drop   bool
	body   []byte
	posts  int
}

func (p *remotePeer) handler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/.well-known/ocm":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(p.disco)
	case strings.HasSuffix(r.URL.Path, "/shares"):
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		p.mu.Lock()
		p.body = body
		p.posts++
		status := p.status
		drop := p.drop
		p.mu.Unlock()
		if drop {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "cannot drop connection", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				return
			}
			_ = conn.Close()
			return
		}
		if status == 0 {
			status = http.StatusCreated
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusCreated || status == http.StatusOK {
			_, _ = w.Write([]byte(`{"recipientDisplayName":"Marie"}`))
		} else {
			_, _ = w.Write([]byte(`{"error":"rejected"}`))
		}
	default:
		http.NotFound(w, r)
	}
}

func (p *remotePeer) posted() (int, []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.posts, append([]byte(nil), p.body...)
}

type discoverySpec struct {
	fileTargets   []string
	folderTargets []string
	fileReceive   bool
	folderReceive bool
	exchangeToken bool
}

func (s discoverySpec) payload() []byte {
	fileProtos := map[string]any{}
	folderProtos := map[string]any{}
	if s.fileReceive {
		fileProtos["webapp-receive"] = map[string]any{"targets": s.fileTargets}
	}
	if s.folderReceive {
		folderProtos["webapp-receive"] = map[string]any{"targets": s.folderTargets}
	}
	caps := []string{"invites"}
	if s.exchangeToken {
		caps = append(caps, "exchange-token")
	}
	raw, err := json.Marshal(wellknown.OcmDiscoveryData{
		Enabled:    true,
		APIVersion: "1.3.0",
		Endpoint:   "https://remote.example/ocm",
		Provider:   "remote",
		ResourceTypes: []wellknown.ResourceTypes{
			{Name: "file", ShareTypes: []string{"user"}, Protocols: fileProtos},
			{Name: "folder", ShareTypes: []string{"user"}, Protocols: folderProtos},
		},
		Capabilities: caps,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func capableDiscovery() discoverySpec {
	return discoverySpec{
		fileTargets:   []string{"blank"},
		folderTargets: []string{"blank"},
		fileReceive:   true,
		folderReceive: true,
		exchangeToken: true,
	}
}

type shareFixture struct {
	offer         bool
	name          string
	endpoint      string
	methods       []*ocm.AccessMethod
	resource      providerpb.ResourceType
	disco         discoverySpec
	shareID       string
	postStatus    int
	dropPost      bool
	expectInvalid bool
	marker        string
	logs          *bytes.Buffer
}

func (f shareFixture) run(
	t *testing.T,
) (*ocmd.NewShareRequest, *ocm.Share, *capturingRepo, *ocm.CreateOCMShareResponse) {
	t.Helper()
	logs := f.logs
	if logs == nil {
		logs = &bytes.Buffer{}
	}
	peer := &remotePeer{
		disco:  f.disco.payload(),
		status: f.postStatus,
		drop:   f.dropPost,
	}
	srv := httptest.NewServer(http.HandlerFunc(peer.handler))
	t.Cleanup(srv.Close)

	repo := &capturingRepo{nextID: f.shareID}
	if repo.nextID == "" {
		repo.nextID = "opaque-share"
	}
	endpoint := f.endpoint
	if f.offer && endpoint == "" {
		endpoint = testOpener
	}
	name := f.name
	if f.offer && name == "" {
		name = testAppName
	}
	svc := &service{
		conf: &config{
			ProviderDomain: "sender.example",
			WebDAVEndpoint: testWebDAVRoot,
			OfferWebapp:    f.offer,
			WebappName:     name,
			WebAppEndpoint: endpoint,
		},
		repo:   repo,
		client: ocmd.NewClient(2*time.Second, true),
	}
	info := &providerpb.ResourceInfo{
		Path: "/files/notes.txt",
		Type: f.resource,
		Owner: &userpb.UserId{
			OpaqueId: "einstein",
			Idp:      "sender.example",
		},
	}
	if f.resource == providerpb.ResourceType_RESOURCE_TYPE_CONTAINER {
		info.Path = "/files/notes-dir"
	}
	if f.resource == 0 {
		info.Type = providerpb.ResourceType_RESOURCE_TYPE_FILE
	}
	stampGateway(&statGateway{info: info})

	user := &userpb.User{
		Id:          &userpb.UserId{OpaqueId: "einstein", Idp: "sender.example"},
		DisplayName: "Albert",
	}
	logger := zerolog.New(logs)
	ctx := appctx.ContextSetUser(context.Background(), user)
	ctx = appctx.WithLogger(ctx, &logger)
	req := &ocm.CreateOCMShareRequest{
		ResourceId: &providerpb.ResourceId{StorageId: "storage", OpaqueId: "resource"},
		Grantee: &providerpb.Grantee{
			Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerpb.Grantee_UserId{
				UserId: &userpb.UserId{OpaqueId: "marie", Idp: "remote.example"},
			},
		},
		RecipientMeshProvider: &ocmprovider.ProviderInfo{
			Domain: "remote.example",
			Services: []*ocmprovider.Service{
				{
					Endpoint: &ocmprovider.ServiceEndpoint{
						Type: &ocmprovider.ServiceType{Name: "OCM"},
						Path: srv.URL + "/ocm",
					},
				},
			},
		},
		AccessMethods: f.methods,
	}
	resp, err := svc.CreateOCMShare(ctx, req)
	f.assertNoMarker(t, logs, resp)
	if err != nil {
		t.Fatalf("CreateOCMShare returned transport error %v", err)
	}
	if f.expectInvalid {
		if resp == nil || resp.Status == nil || resp.Status.Code != rpc.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("status = %#v, want INVALID_ARGUMENT", statusOf(resp))
		}
		if resp.Status.Message != invalidShareAccessMethodsText {
			t.Fatalf("message = %q, want %q", resp.Status.Message, invalidShareAccessMethodsText)
		}
		posts, body := peer.posted()
		if posts != 0 {
			t.Fatalf("remote posts = %d, want 0 body=%s", posts, body)
		}
		if repo.storeN != 0 || repo.stored != nil {
			t.Fatal("invalid share was stored")
		}
		return nil, nil, repo, resp
	}
	if f.dropPost || f.postStatus == http.StatusBadRequest {
		want := codes.InvalidArgument
		if f.dropPost {
			want = codes.Internal
		}
		remoteErr := grpcErrFromCS3(statusOf(resp))
		if got := status.Code(remoteErr); got != want {
			t.Fatalf("grpc status = %s, want %s, cs3 = %#v", got, want, statusOf(resp))
		}
		posts, body := peer.posted()
		if posts != 1 {
			t.Fatalf("remote posts = %d, want 1 body=%s", posts, body)
		}
		if repo.storeN != 0 || repo.stored != nil {
			t.Fatalf("store count = %d, stored set = %t, want 0 and unset", repo.storeN, repo.stored != nil)
		}
		return nil, nil, repo, resp
	}
	if resp.Status.Code != rpc.Code_CODE_OK {
		t.Fatalf("status %s: %s", resp.Status.Code, resp.Status.Message)
	}
	posts, body := peer.posted()
	if posts != 1 {
		t.Fatalf("remote posts = %d, want 1", posts)
	}
	var wire ocmd.NewShareRequest
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatalf("unmarshal wire share: %v body=%s", err, body)
	}
	if repo.stored == nil {
		t.Fatal("expected a stored share")
	}
	return &wire, repo.stored, repo, resp
}

func (f shareFixture) assertNoMarker(t *testing.T, logs *bytes.Buffer, resp *ocm.CreateOCMShareResponse) {
	t.Helper()
	if f.marker == "" {
		return
	}
	if strings.Contains(logs.String(), f.marker) {
		t.Fatalf("logs contain secret marker: %s", logs.String())
	}
	if resp != nil && resp.Status != nil && strings.Contains(resp.Status.Message, f.marker) {
		t.Fatalf("status message leaked marker: %s", resp.Status.Message)
	}
}

func statusOf(resp *ocm.CreateOCMShareResponse) *rpc.Status {
	if resp == nil {
		return nil
	}
	return resp.Status
}

// grpcErrFromCS3 maps the CS3 status CreateOCMShare actually returned.
// Remote NewShare failures come back as a nil Go error, so status.Code on
// that error is OK. Only the returned code is translated; unknown codes stay
// Unknown and cannot satisfy InvalidArgument or Internal.
func grpcErrFromCS3(st *rpc.Status) error {
	if st == nil {
		return status.Error(codes.Unknown, "nil cs3 status")
	}
	var code codes.Code
	switch st.Code {
	case rpc.Code_CODE_INVALID_ARGUMENT:
		code = codes.InvalidArgument
	case rpc.Code_CODE_INTERNAL:
		code = codes.Internal
	default:
		code = codes.Unknown
	}
	return status.Error(code, st.Message)
}

func viewerPerm() *providerpb.ResourcePermissions {
	return &providerpb.ResourcePermissions{InitiateFileDownload: true}
}

func webdavMethod() *ocm.AccessMethod {
	return webdavMethodWith([]string{})
}

func webdavMethodWith(reqs []string) *ocm.AccessMethod {
	return share.NewWebDavAccessMethod(
		viewerPerm(),
		[]ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
		reqs,
	)
}

func webappMethod(name string) *ocm.AccessMethod {
	return webappMethodWith(name, share.DefaultWebappRequirements)
}

func webappMethodWith(name string, reqs []string) *ocm.AccessMethod {
	return share.NewWebappAccessMethod(viewerPerm(), reqs, name)
}

func countWebappStored(methods []*ocm.AccessMethod) int {
	n := 0
	for _, m := range methods {
		webapp, _ := isWebappAccessMethod(m)
		if webapp {
			n++
		}
	}
	return n
}

func countKind(protocols ocmd.Protocols, kind string) int {
	n := 0
	for _, p := range protocols {
		switch p.(type) {
		case *ocmd.Webapp:
			if kind == "webapp" {
				n++
			}
		case *ocmd.WebDAV:
			if kind == "webdav" {
				n++
			}
		}
	}
	return n
}

func webapps(protocols ocmd.Protocols) []*ocmd.Webapp {
	var out []*ocmd.Webapp
	for _, p := range protocols {
		if w, ok := p.(*ocmd.Webapp); ok {
			out = append(out, w)
		}
	}
	return out
}

func webdavs(protocols ocmd.Protocols) []*ocmd.WebDAV {
	var out []*ocmd.WebDAV
	for _, p := range protocols {
		if w, ok := p.(*ocmd.WebDAV); ok {
			out = append(out, w)
		}
	}
	return out
}

func assertNoWebapp(t *testing.T, wire *ocmd.NewShareRequest, stored *ocm.Share) {
	t.Helper()
	if countKind(wire.Protocols, "webapp") != 0 {
		t.Fatalf("wire webapp count = %d, want 0", countKind(wire.Protocols, "webapp"))
	}
	if countWebappStored(stored.AccessMethods) != 0 {
		t.Fatalf("stored webapp count = %d, want 0", countWebappStored(stored.AccessMethods))
	}
}

func assertWebDAVRetained(t *testing.T, wire *ocmd.NewShareRequest, stored *ocm.Share) {
	t.Helper()
	if countKind(wire.Protocols, "webdav") != 1 {
		t.Fatalf("wire webdav count = %d, want 1", countKind(wire.Protocols, "webdav"))
	}
	if len(stored.AccessMethods) < 1 || stored.AccessMethods[0].GetWebdavOptions() == nil {
		t.Fatalf("stored methods = %#v, want a leading webdav method", stored.AccessMethods)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func storedWebapp(t *testing.T, stored *ocm.Share) *ocm.WebappAccessMethod {
	t.Helper()
	var got *ocm.WebappAccessMethod
	for _, m := range stored.AccessMethods {
		opts := m.GetWebappOptions()
		if opts == nil {
			continue
		}
		if got != nil {
			t.Fatal("stored more than one webapp method")
		}
		got = opts
	}
	if got == nil {
		t.Fatal("stored share has no webapp method")
	}
	return got
}

func storedWebDAV(t *testing.T, stored *ocm.Share) *ocm.WebDAVAccessMethod {
	t.Helper()
	var got *ocm.WebDAVAccessMethod
	for _, m := range stored.AccessMethods {
		opts := m.GetWebdavOptions()
		if opts == nil {
			continue
		}
		if got != nil {
			t.Fatal("stored more than one webdav method")
		}
		got = opts
	}
	if got == nil {
		t.Fatal("stored share has no webdav method")
	}
	return got
}
