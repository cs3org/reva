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
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	testOpener      = "https://sender.example/services/ocm/open"
	testWebDAVRoot  = "https://sender.example/webdav"
	testAppName     = "SynthApp"
	testQuotedName  = `Synth "Lab" & <tag>`
	testDriverName  = "offer-gate-test"
	testNoRunDriver = "offer-gate-should-not-run"
)

func TestMain(m *testing.M) {
	revaservice.SetGlobal(testGateways)
	registry.Register(testDriverName, func(context.Context, map[string]any) (share.Repository, error) {
		return &capturingRepo{}, nil
	})
	embedded.Register(testDriverName, func(context.Context, map[string]any) (embedded.Transferrer, error) {
		return noopTransferrer{}, nil
	})
	os.Exit(m.Run())
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
	offer      bool
	name       string
	endpoint   string
	methods    []*ocm.AccessMethod
	resource   providerpb.ResourceType
	disco      discoverySpec
	shareID    string
	postStatus int
	dropPost   bool
}

func (f shareFixture) run(
	t *testing.T,
) (*ocmd.NewShareRequest, *ocm.Share, *capturingRepo, *ocm.CreateOCMShareResponse) {
	t.Helper()
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
	ctx := appctx.ContextSetUser(context.Background(), user)
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
	if f.dropPost || f.postStatus == http.StatusBadRequest {
		if err != nil {
			t.Fatalf("CreateOCMShare returned transport error %v", err)
		}
		if resp == nil || resp.Status.Code == rpc.Code_CODE_OK {
			t.Fatalf("expected remote failure status, got %#v", resp)
		}
		if repo.storeN != 0 || repo.stored != nil {
			t.Fatal("remote failure stored a share")
		}
		return nil, nil, repo, resp
	}
	if err != nil {
		t.Fatalf("CreateOCMShare error: %v", err)
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

func viewerPerm() *providerpb.ResourcePermissions {
	return &providerpb.ResourcePermissions{InitiateFileDownload: true}
}

func webdavMethod() *ocm.AccessMethod {
	return share.NewWebDavAccessMethod(
		viewerPerm(),
		[]ocm.AccessType{ocm.AccessType_ACCESS_TYPE_REMOTE},
		[]string{},
	)
}

func webappMethod(name string) *ocm.AccessMethod {
	return share.NewWebappAccessMethod(viewerPerm(), share.DefaultWebappRequirements, name)
}

func countWebappStored(methods []*ocm.AccessMethod) int {
	n := 0
	for _, m := range methods {
		if isWebappAccessMethod(m) {
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

func TestFreshAccessMethodsNilAndEmpty(t *testing.T) {
	svc := &service{conf: &config{OfferWebapp: true, WebappName: testAppName}}
	if got := svc.freshAccessMethods(nil, true, true); got == nil || len(got) != 0 {
		t.Fatalf("nil request methods -> %#v, want empty slice", got)
	}
	if got := svc.freshAccessMethods([]*ocm.AccessMethod{}, true, true); got == nil || len(got) != 0 {
		t.Fatalf("empty request methods -> %#v, want empty slice", got)
	}
	got := svc.freshAccessMethods([]*ocm.AccessMethod{nil, nil}, true, true)
	if len(got) != 0 {
		t.Fatalf("nil elements produced %#v", got)
	}
}

func TestOfferWebappConfigDecodeAndDisabledStartup(t *testing.T) {
	var decoded config
	err := cfg.Decode(map[string]any{
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"offer_webapp":    true,
		"webapp_name":     testAppName,
		"webapp_endpoint": testOpener,
		"enable_webapp":   false,
	}, &decoded)
	if err != nil {
		t.Fatalf("decode enabled offer: %v", err)
	}
	if !decoded.OfferWebapp || decoded.WebappName != testAppName || decoded.WebAppEndpoint != testOpener {
		t.Fatalf("decoded config = %#v", decoded)
	}

	var defaults config
	err = cfg.Decode(map[string]any{
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"enable_webapp":   true,
	}, &defaults)
	if err != nil {
		t.Fatalf("decode disabled offer: %v", err)
	}
	if defaults.OfferWebapp || defaults.WebappName != "" || defaults.WebAppEndpoint != "" {
		t.Fatalf("disabled defaults = %#v, want false and empty fields", defaults)
	}

	svc, err := New(context.Background(), map[string]any{
		"driver":          testDriverName,
		"embedded_driver": testDriverName,
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"enable_webapp":   true,
	})
	if err != nil {
		t.Fatalf("disabled New: %v", err)
	}
	got := svc.(*service)
	if got.conf.OfferWebapp || got.conf.WebappName != "" || got.conf.WebAppEndpoint != "" {
		t.Fatalf("disabled service config = %#v", got.conf)
	}
}

func TestOfferWebappEnabledStartup(t *testing.T) {
	svc, err := New(context.Background(), map[string]any{
		"driver":          testDriverName,
		"embedded_driver": testDriverName,
		"gatewaysvc":      "127.0.0.1:9142",
		"provider_domain": "sender.example",
		"webdav_endpoint": testWebDAVRoot,
		"offer_webapp":    true,
		"webapp_name":     testAppName,
		"webapp_endpoint": testOpener,
	})
	if err != nil {
		t.Fatalf("enabled New: %v", err)
	}
	if svc == nil {
		t.Fatal("enabled New returned nil service")
	}
	got := svc.(*service)
	if !got.conf.OfferWebapp || got.conf.WebappName != testAppName || got.conf.WebAppEndpoint != testOpener {
		t.Fatalf("enabled service config = %#v", got.conf)
	}
}

func TestOfferWebappInitializationRejectsInvalidConfig(t *testing.T) {
	var driverCalled bool
	registry.Register(testNoRunDriver, func(context.Context, map[string]any) (share.Repository, error) {
		driverCalled = true
		return &capturingRepo{}, nil
	})
	embedded.Register(testNoRunDriver, func(context.Context, map[string]any) (embedded.Transferrer, error) {
		return noopTransferrer{}, nil
	})

	cases := []struct {
		name   string
		cfg    map[string]any
		substr string
	}{
		{
			name: "missing name",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_endpoint": testOpener,
			},
			substr: "webapp_name",
		},
		{
			name: "blank name",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     "   ",
				"webapp_endpoint": testOpener,
			},
			substr: "webapp_name",
		},
		{
			name: "missing endpoint",
			cfg: map[string]any{
				"offer_webapp": true,
				"webapp_name":  testAppName,
			},
			substr: "webapp_endpoint",
		},
		{
			name: "relative endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "/services/ocm/open",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "hostless endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "http:///open",
			},
			substr: "webapp_endpoint",
		},
		{
			name: "malformed endpoint",
			cfg: map[string]any{
				"offer_webapp":    true,
				"webapp_name":     testAppName,
				"webapp_endpoint": "http://[",
			},
			substr: "webapp_endpoint",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			driverCalled = false
			input := map[string]any{
				"driver":          testNoRunDriver,
				"embedded_driver": testNoRunDriver,
				"gatewaysvc":      "127.0.0.1:9142",
				"provider_domain": "sender.example",
				"webdav_endpoint": testWebDAVRoot,
			}
			for k, v := range tt.cfg {
				input[k] = v
			}
			svc, err := New(context.Background(), input)
			if err == nil {
				t.Fatal("expected initialization error")
			}
			if svc != nil {
				t.Fatal("expected no service on invalid config")
			}
			if !strings.Contains(err.Error(), tt.substr) {
				t.Fatalf("error %q does not mention %s", err.Error(), tt.substr)
			}
			if driverCalled {
				t.Fatal("invalid config reached the share repository")
			}
		})
	}
}

func TestCreateOCMShareOfferGate(t *testing.T) {
	graphMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("")}
	ocsMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("")}
	cliMethods := []*ocm.AccessMethod{webdavMethod(), webappMethod("not-configured")}

	cases := []struct {
		name       string
		offer      bool
		methods    []*ocm.AccessMethod
		disco      discoverySpec
		resource   providerpb.ResourceType
		wantWebapp int
		wantWebdav int
		appName    string
		endpoint   string
	}{
		{
			name:       "gate absent remote capable",
			offer:      false,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "gate false remote capable",
			offer:      false,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("Other")},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "gate true both capabilities",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "gate true no webapp candidate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod()},
			disco:      capableDiscovery(),
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true missing webapp-receive",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true targets omit blank",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"iframe"},
				folderTargets: []string{"iframe"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:  "gate true missing exchange-token",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod(""),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"blank"},
				folderTargets: []string{"blank"},
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "graph shaped empty name",
			offer:      true,
			methods:    graphMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "ocs shaped empty name",
			offer:      true,
			methods:    ocsMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "cli shaped different name",
			offer:      true,
			methods:    cliMethods,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:  "every webapp candidate removed",
			offer: true,
			methods: []*ocm.AccessMethod{
				webdavMethod(),
				webappMethod("one"),
				webappMethod("two"),
			},
			disco: discoverySpec{
				fileReceive:   true,
				folderReceive: true,
				fileTargets:   []string{"blank"},
				folderTargets: []string{"blank"},
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:       "file uses same predicate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource:   providerpb.ResourceType_RESOURCE_TYPE_FILE,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:       "folder uses same predicate",
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource:   providerpb.ResourceType_RESOURCE_TYPE_CONTAINER,
			disco:      capableDiscovery(),
			wantWebapp: 1,
			wantWebdav: 1,
			appName:    testAppName,
			endpoint:   testOpener,
		},
		{
			name:     "file ignored when only folder can receive",
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource: providerpb.ResourceType_RESOURCE_TYPE_FILE,
			disco: discoverySpec{
				folderReceive: true,
				folderTargets: []string{"blank"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
		{
			name:     "folder ignored when only file can receive",
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			resource: providerpb.ResourceType_RESOURCE_TYPE_CONTAINER,
			disco: discoverySpec{
				fileReceive:   true,
				fileTargets:   []string{"blank"},
				exchangeToken: true,
			},
			wantWebapp: 0,
			wantWebdav: 1,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			before := proto.Clone(&ocm.CreateOCMShareRequest{AccessMethods: tt.methods}).(*ocm.CreateOCMShareRequest)
			wire, stored, _, _ := shareFixture{
				offer:    tt.offer,
				methods:  tt.methods,
				disco:    tt.disco,
				resource: tt.resource,
			}.run(t)
			if countKind(wire.Protocols, "webapp") != tt.wantWebapp {
				t.Fatalf("wire webapp = %d, want %d", countKind(wire.Protocols, "webapp"), tt.wantWebapp)
			}
			if countWebappStored(stored.AccessMethods) != tt.wantWebapp {
				t.Fatalf("stored webapp = %d, want %d", countWebappStored(stored.AccessMethods), tt.wantWebapp)
			}
			if countKind(wire.Protocols, "webdav") != tt.wantWebdav {
				t.Fatalf("wire webdav = %d, want %d", countKind(wire.Protocols, "webdav"), tt.wantWebdav)
			}
			if !proto.Equal(before, &ocm.CreateOCMShareRequest{AccessMethods: tt.methods}) {
				t.Fatalf("request methods changed: before=%v after=%v", before.AccessMethods, tt.methods)
			}
			if tt.wantWebdav == 1 {
				assertWebDAVRetained(t, wire, stored)
			}
			if tt.wantWebapp == 0 {
				assertNoWebapp(t, wire, stored)
				return
			}
			got := webapps(wire.Protocols)
			if len(got) != 1 {
				t.Fatalf("wire webapps = %d", len(got))
			}
			if got[0].AppName != tt.appName {
				t.Fatalf("wire appName = %q, want %q", got[0].AppName, tt.appName)
			}
			if got[0].URI != tt.endpoint {
				t.Fatalf("wire uri = %q, want %q", got[0].URI, tt.endpoint)
			}
			if !slicesEqual(got[0].Permissions, []string{"read"}) {
				t.Fatalf("wire permissions = %#v, want %#v", got[0].Permissions, []string{"read"})
			}
			if !slicesEqual(got[0].Targets, share.DefaultWebappTargets) {
				t.Fatalf("wire targets = %#v", got[0].Targets)
			}
			if !slicesEqual(got[0].Requirements, share.DefaultWebappRequirements) {
				t.Fatalf("wire requirements = %#v", got[0].Requirements)
			}
			storedWebapp := 0
			for _, m := range stored.AccessMethods {
				opts := m.GetWebappOptions()
				if opts == nil {
					continue
				}
				storedWebapp++
				if opts.AppName != tt.appName {
					t.Fatalf("stored appName = %q, want %q", opts.AppName, tt.appName)
				}
				if !slicesEqual(opts.Requirements, share.DefaultWebappRequirements) {
					t.Fatalf("stored requirements = %#v", opts.Requirements)
				}
			}
			if storedWebapp != 1 {
				t.Fatalf("stored webapp methods = %d", storedWebapp)
			}
		})
	}
}

func TestCreateOCMShareCallerAliasUnchanged(t *testing.T) {
	shared := webappMethod("incoming-name")
	methods := []*ocm.AccessMethod{webdavMethod(), shared, shared}
	beforeName := shared.GetWebappOptions().GetAppName()
	beforePerms := shared.GetWebappOptions().GetPermissions()
	wire, stored, _, _ := shareFixture{
		offer:   true,
		methods: methods,
		disco:   capableDiscovery(),
	}.run(t)
	if shared.GetWebappOptions().GetAppName() != beforeName {
		t.Fatalf("aliased request appName = %q, want %q", shared.GetWebappOptions().GetAppName(), beforeName)
	}
	if shared.GetWebappOptions().GetPermissions() != beforePerms {
		t.Fatal("aliased request permissions pointer changed")
	}
	if len(methods) != 3 {
		t.Fatalf("request len = %d, want 3", len(methods))
	}
	if countKind(wire.Protocols, "webapp") != 1 {
		t.Fatalf("wire webapp protocols = %d, want 1 (map key collapses duplicates)", countKind(wire.Protocols, "webapp"))
	}
	storedWebapp := 0
	for _, m := range stored.AccessMethods {
		if !isWebappAccessMethod(m) {
			continue
		}
		storedWebapp++
		if m == shared || m.GetWebappOptions() == shared.GetWebappOptions() {
			t.Fatal("stored webapp aliases the request method")
		}
		if m.GetWebappOptions().AppName != testAppName {
			t.Fatalf("stored appName = %q", m.GetWebappOptions().AppName)
		}
	}
	if storedWebapp != 2 {
		t.Fatalf("stored webapp methods = %d, want 2 distinct clones", storedWebapp)
	}
	if webapps(wire.Protocols)[0].AppName != testAppName {
		t.Fatalf("wire appName = %q", webapps(wire.Protocols)[0].AppName)
	}
}

func TestCreateOCMShareWireAndStoreMatch(t *testing.T) {
	wire, stored, _, _ := shareFixture{
		offer: false,
		methods: []*ocm.AccessMethod{
			webdavMethod(),
			webappMethod(""),
		},
		disco: capableDiscovery(),
	}.run(t)
	assertNoWebapp(t, wire, stored)
	assertWebDAVRetained(t, wire, stored)
	if len(stored.AccessMethods) != 1 {
		t.Fatalf("stored methods = %d, want the webdav-only offer", len(stored.AccessMethods))
	}
	if countKind(wire.Protocols, "webdav") != 1 || countKind(wire.Protocols, "webapp") != 0 {
		t.Fatalf("wire protocols = %#v", wire.Protocols)
	}
}

func TestCreateOCMShareOpenerIgnoresOpaqueID(t *testing.T) {
	ids := []string{"opaque-aaa", "opaque-bbb"}
	var uris []string
	for _, id := range ids {
		wire, _, _, _ := shareFixture{
			offer:   true,
			shareID: id,
			methods: []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:   capableDiscovery(),
		}.run(t)
		got := webapps(wire.Protocols)
		if len(got) != 1 {
			t.Fatalf("id %s webapps = %d", id, len(got))
		}
		if got[0].URI != testOpener {
			t.Fatalf("id %s uri = %q", id, got[0].URI)
		}
		if strings.Contains(got[0].URI, id) || strings.Contains(got[0].URI, "/lab") {
			t.Fatalf("uri %q contains share id or /lab", got[0].URI)
		}
		uris = append(uris, got[0].URI)
	}
	if uris[0] != uris[1] {
		t.Fatalf("openers differ: %q vs %q", uris[0], uris[1])
	}
}

func TestCreateOCMShareRemoteFailureDoesNotStore(t *testing.T) {
	t.Run("rejection", func(t *testing.T) {
		shareFixture{
			offer:      true,
			methods:    []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:      capableDiscovery(),
			postStatus: http.StatusBadRequest,
		}.run(t)
	})
	t.Run("transport", func(t *testing.T) {
		shareFixture{
			offer:    true,
			methods:  []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
			disco:    capableDiscovery(),
			dropPost: true,
		}.run(t)
	})
}

func TestCreateOCMShareConfiguredNameEscapesOnWire(t *testing.T) {
	wire, stored, _, _ := shareFixture{
		offer:   true,
		name:    testQuotedName,
		methods: []*ocm.AccessMethod{webdavMethod(), webappMethod("")},
		disco:   capableDiscovery(),
	}.run(t)
	got := webapps(wire.Protocols)
	if len(got) != 1 || got[0].AppName != testQuotedName {
		t.Fatalf("wire appName = %#v, want %q", got, testQuotedName)
	}
	if stored.AccessMethods[1].GetWebappOptions().AppName != testQuotedName {
		t.Fatalf("stored appName = %q", stored.AccessMethods[1].GetWebappOptions().AppName)
	}
}

func TestEmptyLegacyWebappNameOmitted(t *testing.T) {
	svc := &service{conf: &config{WebAppEndpoint: testOpener}}
	protocol := svc.getWebappProtocol(&ocm.Share{Token: "secret"}, &ocm.AccessMethod_WebappOptions{
		WebappOptions: &ocm.WebappAccessMethod{
			Permissions:  viewerPerm(),
			Requirements: share.DefaultWebappRequirements,
		},
	})
	raw, err := json.Marshal(protocol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "appName") {
		t.Fatalf("empty appName was serialized: %s", raw)
	}
	if protocol.URI != testOpener {
		t.Fatalf("uri = %q", protocol.URI)
	}
	if !slicesEqual(protocol.Targets, []string{"blank"}) {
		t.Fatalf("targets = %#v", protocol.Targets)
	}
	if !slicesEqual(protocol.Requirements, share.DefaultWebappRequirements) {
		t.Fatalf("requirements = %#v", protocol.Requirements)
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
