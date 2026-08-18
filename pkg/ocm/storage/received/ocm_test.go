package ocm

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ReneKroon/ttlcache/v2"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/studio-b12/gowebdav"
	"google.golang.org/grpc"
)

// ocmDiscoveryServer starts a local httptest.Server that answers /.well-known/ocm
// with a minimal OcmDiscoveryData payload advertising the given protocol for the
// given resource type. The server's own URL is used as the OCM endpoint so that
// any URL constructed from the discovery response also resolves locally.
// Callers must call srv.Close() when done (typically via t.Cleanup).
func ocmDiscoveryServer(t *testing.T, proto, resType string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	// We need a two-step setup: register the handler before the server starts,
	// but reference srv.URL inside the handler. Use a pointer so the closure
	// captures the final value after httptest.NewServer returns.
	var srv *httptest.Server
	mux.HandleFunc("/.well-known/ocm", func(w http.ResponseWriter, r *http.Request) {
		endpoint := "http://" + r.Host
		disco := wellknown.OcmDiscoveryData{
			Endpoint: endpoint,
			ResourceTypes: []wellknown.ResourceTypes{
				{
					Name: resType,
					Protocols: map[string]any{
						proto: "/remote.php/dav/ocm",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(disco)
	})
	// Also serve /ocm/token so tests that exercise the code-flow path and happen
	// to hit this server for token exchange get a sensible default response.
	mux.HandleFunc("/ocm/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "mock-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// --- helpers ---

func TestShareInfoFromPath(t *testing.T) {
	id, rel := shareInfoFromPath("/share123/sub/file.txt")
	if id.OpaqueId != "share123" {
		t.Errorf("shareID: got %q, want share123", id.OpaqueId)
	}
	if rel != "/sub/file.txt" {
		t.Errorf("rel: got %q, want /sub/file.txt", rel)
	}
}

func TestShareInfoFromPath_RootOnly(t *testing.T) {
	id, rel := shareInfoFromPath("/share-only")
	if id.OpaqueId != "share-only" {
		t.Errorf("shareID: got %q, want share-only", id.OpaqueId)
	}
	if rel != "/" {
		t.Errorf("rel: got %q, want /", rel)
	}
}

func TestShareInfoFromReference_PathBased(t *testing.T) {
	ref := &provider.Reference{Path: "/share-abc/docs/readme.md"}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "/docs/readme.md" {
		t.Errorf("rel: got %q, want /docs/readme.md", rel)
	}
}

func TestShareInfoFromReference_ResourceIdWithColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share-abc:sub"},
		Path:       "file.txt",
	}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "sub/file.txt" {
		t.Errorf("rel: got %q, want sub/file.txt", rel)
	}
}

func TestShareInfoFromReference_ResourceIdNoColon(t *testing.T) {
	ref := &provider.Reference{
		ResourceId: &provider.ResourceId{OpaqueId: "share-abc"},
		Path:       "file.txt",
	}
	id, rel := shareInfoFromReference(ref)
	if id.OpaqueId != "share-abc" {
		t.Errorf("shareID: got %q, want share-abc", id.OpaqueId)
	}
	if rel != "file.txt" {
		t.Errorf("rel: got %q, want file.txt", rel)
	}
}

func TestGetWebDAVProtocol_Found(t *testing.T) {
	webdav := &ocmpb.WebDAVProtocol{Uri: "https://remote/dav", SharedSecret: "s3cret"}
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: webdav}},
	}
	got, ok := getWebDAVProtocol(protocols)
	if !ok {
		t.Fatal("expected to find WebDAV protocol")
	}
	if got.Uri != "https://remote/dav" {
		t.Errorf("uri: got %q, want https://remote/dav", got.Uri)
	}
}

func TestGetWebDAVProtocol_NotFound(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{}}},
	}
	_, ok := getWebDAVProtocol(protocols)
	if ok {
		t.Error("expected not to find WebDAV protocol")
	}
}

func TestRequiresExchange_True(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
			Permissions: &ocmpb.SharePermissions{
				Permissions: &provider.ResourcePermissions{Stat: true},
			},
			Requirements: []string{"must-exchange-token"},
		}}},
	}
	if !requiresExchange(protocols) {
		t.Error("expected requiresExchange=true")
	}
}

func TestRequiresExchange_False(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
			Permissions: &ocmpb.SharePermissions{
				Permissions: &provider.ResourcePermissions{Stat: true},
			},
		}}},
	}
	if requiresExchange(protocols) {
		t.Error("expected requiresExchange=false when no requirements")
	}
}

func TestRequiresExchange_NoWebDAV(t *testing.T) {
	protocols := []*ocmpb.Protocol{
		{Term: &ocmpb.Protocol_WebappOptions{WebappOptions: &ocmpb.WebappProtocol{}}},
	}
	if requiresExchange(protocols) {
		t.Error("expected requiresExchange=false when no WebDAV protocol")
	}
}

func TestGetResourceInfo(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getResourceInfo(id, "sub/file.txt")
	want := "share-abc:sub/file.txt"
	if got.OpaqueId != want {
		t.Errorf("OpaqueId: got %q, want %q", got.OpaqueId, want)
	}
}

func TestGetPathFromShareIDAndRelPath(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getPathFromShareIDAndRelPath(id, "sub/file.txt")
	if got != "/share-abc/sub/file.txt" {
		t.Errorf("got %q, want /share-abc/sub/file.txt", got)
	}
}

func TestGetPathFromShareIDAndRelPath_Root(t *testing.T) {
	id := &ocmpb.ShareId{OpaqueId: "share-abc"}
	got := getPathFromShareIDAndRelPath(id, "")
	if got != "/share-abc" {
		t.Errorf("got %q, want /share-abc", got)
	}
}

// --- fakes and mocks ---

type fakeFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (f *fakeFileInfo) Name() string      { return f.name }
func (f *fakeFileInfo) Size() int64       { return f.size }
func (f *fakeFileInfo) Mode() fs.FileMode { return f.mode }
func (f *fakeFileInfo) ModTime() time.Time {
	if f.modTime.IsZero() {
		return time.Unix(1700000000, 0)
	}
	return f.modTime
}
func (f *fakeFileInfo) IsDir() bool { return f.isDir }
func (f *fakeFileInfo) Sys() any    { return nil }

type mockReceivedGateway struct {
	gateway.GatewayAPIClient
	shares []*ocmpb.ReceivedShare
	calls  int
}

func (m *mockReceivedGateway) GetReceivedOCMShare(_ context.Context, _ *ocmpb.GetReceivedOCMShareRequest, _ ...grpc.CallOption) (*ocmpb.GetReceivedOCMShareResponse, error) {
	idx := m.calls
	if idx >= len(m.shares) {
		idx = len(m.shares) - 1
	}
	m.calls++
	return &ocmpb.GetReceivedOCMShareResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Share:  m.shares[idx],
	}, nil
}

// testReceivedShare builds a minimal ReceivedShare whose Creator.Idp and WebDAV
// URI host both point at senderAddr (e.g. "127.0.0.1:12345"), so that any
// discovery call the driver makes — whether it derives the target from the
// WebDAV URI or from Creator.Idp — hits the local httptest.Server instead of
// the real internet.
func testReceivedShare(senderAddr, id string, isFile bool) *ocmpb.ReceivedShare {
	srt := ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_CONTAINER
	if isFile {
		srt = ocmpb.SharedResourceType_SHARE_RESOURCE_TYPE_FILE
	}
	return &ocmpb.ReceivedShare{
		Id:   &ocmpb.ShareId{OpaqueId: id},
		Name: "shared-doc.txt",
		Creator: &userpb.UserId{
			OpaqueId: "creator",
			Idp:      senderAddr,
		},
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: &userpb.UserId{
					OpaqueId: "receiver",
					Idp:      "nextcloud1.docker",
				},
			},
		},
		SharedResourceType: srt,
		Protocols: []*ocmpb.Protocol{
			{Term: &ocmpb.Protocol_WebdavOptions{WebdavOptions: &ocmpb.WebDAVProtocol{
				Uri:          "http://" + senderAddr + "/remote.php/dav/ocm/" + id,
				SharedSecret: "secret",
				Permissions: &ocmpb.SharePermissions{
					Permissions: &provider.ResourcePermissions{
						Stat:                 true,
						InitiateFileDownload: true,
						InitiateFileUpload:   true,
					},
				},
			}}},
		},
	}
}

func newTestReceivedDriver() *driver {
	disco := ttlcache.NewCache()
	_ = disco.SetTTL(5 * time.Minute)
	return &driver{
		ccache:         ttlcache.NewCache(),
		discoveryCache: disco,
		ocmClient:      ocmd.NewClient(10*time.Second, true),
	}
}

func testCodeFlowReceivedShare(senderAddr, baseURL string) *ocmpb.ReceivedShare {
	share := testReceivedShare(senderAddr, "share-abc", false)
	webdav := share.Protocols[0].GetWebdavOptions()
	webdav.Uri = baseURL + "/remote.php/dav/ocm/share-abc"
	webdav.Requirements = []string{"must-exchange-token"}
	return share
}

// --- receiver client ID tests ---
// These tests don't trigger network calls; they use a static senderAddr since
// the share fields are never passed to the OCM client here.

func TestReceiverClientIDPrefersContextUserIDP(t *testing.T) {
	share := testReceivedShare("sender.example.com", "share-abc", false)
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Id: &userpb.UserId{OpaqueId: "local-user", Idp: "local-context.example"},
	})

	got := receiverClientID(ctx, share)
	if got != "local-context.example" {
		t.Errorf("got %q, want local-context.example", got)
	}
}

func TestReceiverClientIDFallsBackToShareGranteeIDP(t *testing.T) {
	share := testReceivedShare("sender.example.com", "share-abc", false)

	got := receiverClientID(context.Background(), share)
	if got != "nextcloud1.docker" {
		t.Errorf("got %q, want nextcloud1.docker", got)
	}
}

func TestReceiverClientIDReturnsEmptyWhenUnavailable(t *testing.T) {
	share := testReceivedShare("sender.example.com", "share-abc", false)
	share.Grantee = nil

	got := receiverClientID(context.Background(), share)
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestReceiverClientIDWithLookupFallsBackToGatewayUserIDP(t *testing.T) {
	share := testReceivedShare("sender.example.com", "share-abc", false)
	share.Grantee.GetUserId().Idp = ""

	got := receiverClientIDWithLookup(context.Background(), share, func(_ context.Context, userID *userpb.UserId) string {
		if userID.GetOpaqueId() != "receiver" {
			t.Fatalf("lookup user id: got %q, want receiver", userID.GetOpaqueId())
		}
		return "local-gateway.example"
	})
	if got != "local-gateway.example" {
		t.Errorf("got %q, want local-gateway.example", got)
	}
}

func TestReceiverClientIDWithLookupSkipsGatewayWhenShareAlreadyHasIDP(t *testing.T) {
	share := testReceivedShare("sender.example.com", "share-abc", false)
	lookupCalled := false

	got := receiverClientIDWithLookup(context.Background(), share, func(_ context.Context, _ *userpb.UserId) string {
		lookupCalled = true
		return "unexpected.example"
	})
	if got != "nextcloud1.docker" {
		t.Errorf("got %q, want nextcloud1.docker", got)
	}
	if lookupCalled {
		t.Error("expected lookup not to be called when share grantee already has an idp")
	}
}

// --- discovery / token-exchange tests ---
// All of these spin up a local httptest.Server and thread its address through
// both Creator.Idp and the WebDAV URI host via testReceivedShare /
// testCodeFlowReceivedShare, so no real outbound DNS lookups are made.

func TestGetTokenEndpointCachesDiscovery(t *testing.T) {
	discoveryCalls := 0
	var unexpectedPath string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/ocm" {
			unexpectedPath = r.URL.Path
			http.Error(w, "unexpected path", http.StatusInternalServerError)
			return
		}
		discoveryCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":       true,
			"apiVersion":    "1.2.0",
			"endPoint":      srv.URL + "/ocm",
			"provider":      "reva",
			"resourceTypes": []any{},
			"capabilities":  []string{"exchange-token"},
			"tokenEndPoint": srv.URL + "/ocm/token",
		})
	}))
	defer srv.Close()

	d := newTestReceivedDriver()
	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)

	got1, err := d.getTokenEndpoint(context.Background(), share)
	if err != nil {
		t.Fatalf("getTokenEndpoint first call returned error: %v", err)
	}
	got2, err := d.getTokenEndpoint(context.Background(), share)
	if err != nil {
		t.Fatalf("getTokenEndpoint second call returned error: %v", err)
	}
	want := srv.URL + "/ocm/token"
	if got1 != want || got2 != want {
		t.Fatalf("getTokenEndpoint() = %q, %q, want %q", got1, got2, want)
	}
	if discoveryCalls != 1 {
		t.Fatalf("expected discovery to be called once, got %d", discoveryCalls)
	}
	if unexpectedPath != "" {
		t.Fatalf("unexpected path: got %q, want /.well-known/ocm", unexpectedPath)
	}
}

func TestGetTokenEndpointRequiresDiscoveryTokenEndpoint(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":       true,
			"apiVersion":    "1.2.0",
			"endPoint":      srv.URL + "/ocm",
			"provider":      "reva",
			"resourceTypes": []any{},
			"capabilities":  []string{"exchange-token"},
			// intentionally omitting "tokenEndPoint"
		})
	}))
	defer srv.Close()

	d := newTestReceivedDriver()
	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)

	_, err := d.getTokenEndpoint(context.Background(), share)
	if err == nil {
		t.Fatal("expected error when discovery payload has no tokenEndPoint")
	}
	if _, ok := err.(errtypes.IsNotFound); !ok {
		t.Fatalf("expected NotFound error, got %T: %v", err, err)
	}
}

func TestUploadAuthCodeFlowExchangesBearerToken(t *testing.T) {
	var gotCode, gotClientID string
	discoveryCalls := 0
	tokenCalls := 0
	var parseErr error
	var unexpectedPath string
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			discoveryCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"apiVersion":    "1.2.0",
				"endPoint":      srv.URL + "/ocm",
				"provider":      "reva",
				"resourceTypes": []any{},
				"capabilities":  []string{"exchange-token"},
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case "/ocm/token":
			tokenCalls++
			if err := r.ParseForm(); err != nil {
				parseErr = err
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			gotCode = r.FormValue("code")
			gotClientID = r.FormValue("client_id")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "jwt-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			unexpectedPath = r.URL.Path
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	d := newTestReceivedDriver()
	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)

	got, err := d.uploadAuth(context.Background(), share, share.Protocols[0].GetWebdavOptions().Uri, "exchange-secret", share.GetId())
	if err != nil {
		t.Fatalf("uploadAuth returned error: %v", err)
	}
	if got != "Bearer jwt-tok" {
		t.Fatalf("uploadAuth() = %q, want %q", got, "Bearer jwt-tok")
	}
	if gotCode != "exchange-secret" {
		t.Fatalf("code: got %q, want %q", gotCode, "exchange-secret")
	}
	if gotClientID != "nextcloud1.docker" {
		t.Fatalf("client_id: got %q, want %q", gotClientID, "nextcloud1.docker")
	}
	if discoveryCalls != 1 || tokenCalls != 1 {
		t.Fatalf("expected one discovery call and one token call, got discovery=%d token=%d", discoveryCalls, tokenCalls)
	}
	if parseErr != nil {
		t.Fatalf("ParseForm returned error: %v", parseErr)
	}
	if unexpectedPath != "" {
		t.Fatalf("unexpected path: %q", unexpectedPath)
	}
}

func TestUploadAuthLegacyUsesCachedHeader(t *testing.T) {
	// Legacy path reads from ccache before any discovery; the senderAddr is
	// unused by the code path under test, so a static placeholder is fine here.
	d := newTestReceivedDriver()
	share := testReceivedShare("sender.example.com", "share-abc", false)
	_ = d.ccache.Set(share.GetId().GetOpaqueId(), &cachedClient{authHeader: "Basic cached-auth"})

	got, err := d.uploadAuth(context.Background(), share, share.Protocols[0].GetWebdavOptions().Uri, "legacy-secret", share.GetId())
	if err != nil {
		t.Fatalf("uploadAuth returned error: %v", err)
	}
	if got != "Basic cached-auth" {
		t.Fatalf("uploadAuth() = %q, want %q", got, "Basic cached-auth")
	}
}

func TestWithExchangeStatRetryRetriesAndReturnsFreshShare(t *testing.T) {
	discoveryCalls := 0
	tokenCalls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			discoveryCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"apiVersion":    "1.2.0",
				"endPoint":      srv.URL + "/ocm",
				"provider":      "reva",
				"resourceTypes": []any{},
				"capabilities":  []string{"exchange-token"},
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case "/ocm/token":
			tokenCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "jwt-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	senderAddr := srv.Listener.Addr().String()
	share1 := testCodeFlowReceivedShare(senderAddr, srv.URL)
	share1.Name = "stale-name"
	share2 := testCodeFlowReceivedShare(senderAddr, srv.URL)
	share2.Name = "fresh-name"

	d := newTestReceivedDriver()
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share1, share2}})

	ref := &provider.Reference{Path: "/share-abc/docs"}
	fnCalls := 0
	info, share, rel, err := d.withExchangeStatRetry(context.Background(), ref, func(_ *gowebdav.Client, _ string) (fs.FileInfo, error) {
		fnCalls++
		if fnCalls == 1 {
			return nil, gowebdav.NewPathError("GET", "/docs", http.StatusUnauthorized)
		}
		return &fakeFileInfo{name: "docs", isDir: true}, nil
	})
	if err != nil {
		t.Fatalf("withExchangeStatRetry returned error: %v", err)
	}
	if fnCalls != 2 {
		t.Fatalf("expected fn to be called twice, got %d", fnCalls)
	}
	if discoveryCalls != 1 {
		t.Fatalf("expected discovery to be called once, got %d", discoveryCalls)
	}
	if tokenCalls != 2 {
		t.Fatalf("expected token endpoint to be called twice, got %d", tokenCalls)
	}
	if info == nil || !info.IsDir() {
		t.Fatalf("expected directory info after retry, got %#v", info)
	}
	if share.GetName() != "fresh-name" {
		t.Fatalf("expected fresh share metadata after retry, got %q", share.GetName())
	}
	if rel != "/docs" {
		t.Fatalf("expected rel path /docs, got %q", rel)
	}
}

func TestWithExchangeRetrySecond401ReturnsInvalidCredentials(t *testing.T) {
	discoveryCalls := 0
	tokenCalls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			discoveryCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"apiVersion":    "1.2.0",
				"endPoint":      srv.URL + "/ocm",
				"provider":      "reva",
				"resourceTypes": []any{},
				"capabilities":  []string{"exchange-token"},
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case "/ocm/token":
			tokenCalls++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "jwt-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	senderAddr := srv.Listener.Addr().String()
	share := testCodeFlowReceivedShare(senderAddr, srv.URL)
	d := newTestReceivedDriver()
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share, share}})

	ref := &provider.Reference{Path: "/share-abc/docs"}
	fnCalls := 0
	err := d.withExchangeRetry(context.Background(), ref, func(_ *gowebdav.Client, _ string) error {
		fnCalls++
		return gowebdav.NewPathError("GET", "/docs", http.StatusUnauthorized)
	})
	if err == nil {
		t.Fatal("expected invalid credentials error after second 401")
	}
	if _, ok := err.(errtypes.IsInvalidCredentials); !ok {
		t.Fatalf("expected InvalidCredentials error, got %T: %v", err, err)
	}
	if fnCalls != 2 {
		t.Fatalf("expected fn to be called twice, got %d", fnCalls)
	}
	if discoveryCalls != 1 {
		t.Fatalf("expected discovery to be called once, got %d", discoveryCalls)
	}
	if tokenCalls != 2 {
		t.Fatalf("expected token endpoint to be called twice, got %d", tokenCalls)
	}
}

// --- convertStatToResourceInfo tests ---

func TestConvertStatToResourceInfo_File(t *testing.T) {
	fi := &fakeFileInfo{name: "file.txt", size: 1024}
	// senderAddr is irrelevant: convertStatToResourceInfo never triggers discovery.
	share := testReceivedShare("sender.example.com", "share-abc", true)

	info := convertStatToResourceInfo(fi, share, "sub/file.txt")

	if info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		t.Errorf("type: got %v, want FILE", info.Type)
	}
	// for file shares, the name comes from share.Name
	if info.Name != "shared-doc.txt" {
		t.Errorf("name: got %q, want shared-doc.txt", info.Name)
	}
	if info.Size != 1024 {
		t.Errorf("size: got %d, want 1024", info.Size)
	}
	if info.Path != "/share-abc/sub/file.txt" {
		t.Errorf("path: got %q, want /share-abc/sub/file.txt", info.Path)
	}
	if info.Id.OpaqueId != "share-abc:sub/file.txt" {
		t.Errorf("id: got %q, want share-abc:sub/file.txt", info.Id.OpaqueId)
	}
	if info.Owner.OpaqueId != "creator" {
		t.Errorf("owner: got %q, want creator", info.Owner.OpaqueId)
	}
	if !info.PermissionSet.InitiateFileDownload {
		t.Error("expected InitiateFileDownload permission")
	}
}

func TestConvertStatToResourceInfo_Dir(t *testing.T) {
	fi := &fakeFileInfo{name: "docs", size: 0, isDir: true}
	share := testReceivedShare("sender.example.com", "share-abc", false)

	info := convertStatToResourceInfo(fi, share, "docs")

	if info.Type != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		t.Errorf("type: got %v, want CONTAINER", info.Type)
	}
	// for folder shares, the name comes from FileInfo.Name()
	if info.Name != "docs" {
		t.Errorf("name: got %q, want docs", info.Name)
	}
}

// --- isWebDAV401 tests ---

func TestIsWebDAV401_True(t *testing.T) {
	err := gowebdav.NewPathError("GET", "/test", http.StatusUnauthorized)
	if !isWebDAV401(err) {
		t.Error("expected isWebDAV401=true for 401 PathError")
	}
}

func TestIsWebDAV401_OtherStatus(t *testing.T) {
	err := gowebdav.NewPathError("GET", "/test", http.StatusForbidden)
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for 403 PathError")
	}
}

func TestIsWebDAV401_PlainError(t *testing.T) {
	err := fmt.Errorf("some error")
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for plain error")
	}
}

func TestIsWebDAV401_OsPathError(t *testing.T) {
	err := &os.PathError{Op: "GET", Path: "/test", Err: fmt.Errorf("not a StatusError")}
	if isWebDAV401(err) {
		t.Error("expected isWebDAV401=false for PathError with non-StatusError inner")
	}
}
