package ocm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
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

func newReceivedDriver(t *testing.T, m map[string]any) *driver {
	t.Helper()
	fs, err := New(context.Background(), m)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := fs.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", fs)
	}
	return d
}

func newTestReceivedDriver(t *testing.T) *driver {
	t.Helper()
	return newReceivedDriver(t, map[string]any{
		"ocm_insecure":              true,
		"allow_loopback_federation": true,
	})
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

	d := newTestReceivedDriver(t)
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

	d := newTestReceivedDriver(t)
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

	d := newTestReceivedDriver(t)
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
	d := newTestReceivedDriver(t)
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

	d := newTestReceivedDriver(t)
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
	d := newTestReceivedDriver(t)
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

func writeReceivedWebDAVPropfind(w http.ResponseWriter, isDir bool) {
	resourceType := "<d:resourcetype/>"
	if isDir {
		resourceType = `<d:resourcetype><d:collection/></d:resourcetype>`
	}
	body := `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>/</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>item</d:displayname>
        ` + resourceType + `
        <d:getcontentlength>1</d:getcontentlength>
        <d:getcontenttype>text/plain</d:getcontenttype>
        <d:getetag>"e"</d:getetag>
        <d:getlastmodified>Mon, 01 Jan 2024 00:00:00 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(body))
}

func TestNewReceivedDriverPublicOnly(t *testing.T) {
	d := newReceivedDriver(t, map[string]any{})
	if d.ocmClient == nil {
		t.Fatal("New() did not store an OCM client")
	}
	if d.webdavTransport == nil {
		t.Fatal("New() did not store a WebDAV round tripper")
	}
	tr := client.HTTPTransport(d.webdavTransport)
	if tr == nil {
		t.Fatalf("webdav transport: got %T, want *http.Transport or public-only wrapper", d.webdavTransport)
	}
	if tr.Proxy != nil {
		t.Fatal("received WebDAV round tripper must be public-only and must not use a proxy")
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("default received WebDAV round tripper must verify TLS")
	}
	if d.c.AllowLoopbackFederation {
		t.Fatal("allow_loopback_federation must stay off by default")
	}
	if d.c.OCMClientTimeout != 10 {
		t.Errorf("default ocm_timeout = %d, want 10", d.c.OCMClientTimeout)
	}
}

const (
	receivedProxyChildKey    = "OCM_RECEIVED_TEST_PROXY_CHILD"
	receivedProxyAddrKey     = "OCM_RECEIVED_TEST_PROXY_ADDR"
	receivedProxyTarget      = "https://ocm-target.example"
	receivedProxyCONNECTHost = "ocm-target.example:443"
	receivedDirectTarget     = "https://192.168.1.1:9"
)

// TestReceivedDriverUseEnvProxyWiring checks both received copy-ins by
// observing CONNECT on a local 127.0.0.1:0 listener. Each case runs in a
// child because ProxyFromEnvironment snapshots HTTPS_PROXY once per process.
// Discover and Stat each get their own listener so Discover retries cannot
// mask a missing WebDAV CONNECT. The proxy children target a non-loopback
// hostname so Go will not bypass the proxy. The omitted-key child uses
// RFC1918 so Control rejects before connect.
func TestReceivedDriverUseEnvProxyWiring(t *testing.T) {
	if scenario := os.Getenv(receivedProxyChildKey); scenario != "" {
		runReceivedProxyChild(t, scenario)
		return
	}

	t.Run("ocm_use_env_proxy true sends CONNECT for Discover and Stat", func(t *testing.T) {
		want := "CONNECT " + receivedProxyCONNECTHost

		discoSpy := startReceivedCONNECTListener(t)
		runReceivedProxyChildProcess(t, "proxy-discover", discoSpy.addr())
		if n := countReceivedCONNECT(discoSpy.seenRequests(), want); n < 1 {
			t.Fatalf("Discover CONNECT requests = %q, want at least one %q", discoSpy.seenRequests(), want)
		}

		webdavSpy := startReceivedCONNECTListener(t)
		runReceivedProxyChildProcess(t, "proxy-stat", webdavSpy.addr())
		if n := countReceivedCONNECT(webdavSpy.seenRequests(), want); n < 1 {
			t.Fatalf("Stat CONNECT requests = %q, want at least one %q", webdavSpy.seenRequests(), want)
		}
	})

	t.Run("omitted key dials HTTPS RFC1918 direct", func(t *testing.T) {
		spy := startReceivedCONNECTListener(t)
		runReceivedProxyChildProcess(t, "direct", spy.addr())
		if seen := spy.seenRequests(); len(seen) != 0 {
			t.Fatalf("omitted key observed CONNECT: %q", seen)
		}
	})
}

type receivedCONNECTListener struct {
	ln   net.Listener
	mu   sync.Mutex
	seen []string
}

func startReceivedCONNECTListener(t *testing.T) *receivedCONNECTListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &receivedCONNECTListener{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go s.accept()
	return s
}

func (s *receivedCONNECTListener) accept() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *receivedCONNECTListener) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	req, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return
	}
	s.mu.Lock()
	s.seen = append(s.seen, req.Method+" "+req.Host)
	s.mu.Unlock()
	_, _ = conn.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n"))
}

func (s *receivedCONNECTListener) addr() string {
	return s.ln.Addr().String()
}

func (s *receivedCONNECTListener) seenRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.seen))
	copy(out, s.seen)
	return out
}

func countReceivedCONNECT(seen []string, want string) int {
	n := 0
	for _, got := range seen {
		if got == want {
			n++
		}
	}
	return n
}

func receivedProxyChildEnv(scenario, proxyAddr string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "http_proxy", "https_proxy", "no_proxy", "cgi_no_proxy":
			continue
		}
		if key == receivedProxyChildKey || key == receivedProxyAddrKey {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		receivedProxyChildKey+"="+scenario,
		receivedProxyAddrKey+"="+proxyAddr,
	)
}

func runReceivedProxyChildProcess(t *testing.T, scenario, proxyAddr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		os.Args[0],
		"-test.run=^TestReceivedDriverUseEnvProxyWiring$",
		"-test.v=true",
	)
	cmd.Env = receivedProxyChildEnv(scenario, proxyAddr)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("proxy child %s timed out: %v\n%s", scenario, err, out)
	}
	if err != nil {
		t.Fatalf("proxy child %s failed: %v\n%s", scenario, err, out)
	}
}

func runReceivedProxyChild(t *testing.T, scenario string) {
	t.Helper()
	proxyAddr := os.Getenv(receivedProxyAddrKey)
	if proxyAddr == "" {
		t.Fatal("child missing " + receivedProxyAddrKey)
	}
	t.Setenv("HTTPS_PROXY", "http://"+proxyAddr)

	var d *driver
	var target string
	switch scenario {
	case "direct":
		// Omitted ocm_use_env_proxy keeps Proxy nil, so HTTPS_PROXY must not CONNECT.
		d = newReceivedDriver(t, map[string]any{})
		target = receivedDirectTarget
	case "proxy-discover", "proxy-stat":
		// allow_loopback_federation is required so Control accepts the
		// 127.0.0.1 proxy hop; otherwise CONNECT never reaches the listener.
		d = newReceivedDriver(t, map[string]any{
			"ocm_use_env_proxy":         true,
			"allow_loopback_federation": true,
		})
		target = receivedProxyTarget
	default:
		t.Fatalf("unknown child scenario %q", scenario)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if scenario != "proxy-stat" {
		_, discoErr := d.ocmClient.Discover(ctx, target)
		if scenario == "direct" {
			assertReceivedDirectPolicyError(t, "Discover", discoErr)
		} else if errors.Is(discoErr, client.ErrPolicyViolation) {
			t.Fatalf("true config denied the Discover proxy hop: %v", discoErr)
		}
	}
	if scenario != "proxy-discover" {
		_, statErr := d.newWebDAVClient(target, nil).Stat("")
		if scenario == "direct" {
			assertReceivedDirectPolicyError(t, "Stat", statErr)
		} else if errors.Is(statErr, client.ErrPolicyViolation) {
			t.Fatalf("true config denied the WebDAV proxy hop: %v", statErr)
		}
	}
}

func assertReceivedDirectPolicyError(t *testing.T, op string, err error) {
	t.Helper()
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("omitted key %s error = %v, want ErrPolicyViolation", op, err)
	}
	got := err.Error()
	if !strings.Contains(got, "non-public address") {
		t.Errorf("omitted key %s error = %q, want non-public address", op, got)
	}
	if !strings.Contains(got, "192.168.1.1:9") {
		t.Errorf("omitted key %s error = %q, want 192.168.1.1:9", op, got)
	}
	if strings.Contains(got, "refusing scheme") {
		t.Errorf("omitted key %s error = %q, must not contain refusing scheme", op, got)
	}
}

func TestReceivedDiscoverRejectsLoopbackByDefault(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":       true,
			"endPoint":      "http://" + r.Host + "/ocm",
			"tokenEndPoint": "http://" + r.Host + "/ocm/token",
		})
	}))
	defer srv.Close()

	d := newReceivedDriver(t, map[string]any{})
	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)
	_, err := d.getTokenEndpoint(context.Background(), share)
	if err == nil {
		t.Fatal("expected discovery to refuse loopback")
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("getTokenEndpoint() error = %v, want ErrPolicyViolation", err)
	}
	if hits != 0 {
		t.Fatalf("server hits = %d, want 0", hits)
	}
}

func TestReceivedExchangeTokenRejectsPrivateEndpoint(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"enabled":       true,
			"apiVersion":    "1.2.0",
			"endPoint":      srv.URL + "/ocm",
			"tokenEndPoint": "https://192.168.1.1:9/ocm/token",
		})
	}))
	defer srv.Close()

	d := newTestReceivedDriver(t)
	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)
	endpoint, err := d.getTokenEndpoint(context.Background(), share)
	if err != nil {
		t.Fatalf("getTokenEndpoint returned error: %v", err)
	}
	_, err = d.exchangeAccessToken(context.Background(), share, endpoint, "secret")
	if err == nil {
		t.Fatal("expected token exchange to refuse RFC1918")
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("exchangeAccessToken() error = %v, want ErrPolicyViolation", err)
	}
	got := err.Error()
	if !strings.Contains(got, "non-public address") {
		t.Errorf("exchangeAccessToken() error = %q, want non-public address", got)
	}
	if !strings.Contains(got, "192.168.1.1:9") {
		t.Errorf("exchangeAccessToken() error = %q, want 192.168.1.1:9", got)
	}
	if strings.Contains(got, "refusing scheme") {
		t.Errorf("exchangeAccessToken() error = %q, must not contain refusing scheme", got)
	}
}

func TestWebDAVClientRejectsLoopbackByDefault(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		writeReceivedWebDAVPropfind(w, false)
	}))
	defer srv.Close()

	share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
	d := newReceivedDriver(t, map[string]any{})
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})

	_, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
	if err == nil {
		t.Fatal("expected webdavClient to refuse loopback")
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("webdavClient() error = %v, want ErrPolicyViolation", err)
	}
	if hits != 0 {
		t.Fatalf("server hits = %d, want 0", hits)
	}
}

func TestWebDAVClientLegacyAuthProbes(t *testing.T) {
	const secret = "secret"
	basicHdr := "Basic " + base64.StdEncoding.EncodeToString([]byte(secret+":"))

	t.Run("bearer succeeds", func(t *testing.T) {
		var bearerHits, basicHits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			switch {
			case strings.HasPrefix(auth, "Bearer "):
				bearerHits++
				if auth == "Bearer "+secret {
					writeReceivedWebDAVPropfind(w, false)
					return
				}
			case strings.HasPrefix(auth, "Basic "):
				basicHits++
			}
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})

		_, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
		if err != nil {
			t.Fatalf("webdavClient returned error: %v", err)
		}
		if bearerHits == 0 {
			t.Fatal("expected a bearer Stat probe")
		}
		if basicHits != 0 {
			t.Fatalf("basic hits = %d, want 0", basicHits)
		}
	})

	t.Run("basic fallback after bearer 401", func(t *testing.T) {
		var bearerHits, basicHits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			switch {
			case strings.HasPrefix(auth, "Bearer "):
				bearerHits++
				w.WriteHeader(http.StatusUnauthorized)
				return
			case auth == basicHdr:
				basicHits++
				writeReceivedWebDAVPropfind(w, false)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer srv.Close()

		share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})

		_, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
		if err != nil {
			t.Fatalf("webdavClient returned error: %v", err)
		}
		if bearerHits == 0 {
			t.Fatal("expected a bearer Stat probe before basic fallback")
		}
		if basicHits == 0 {
			t.Fatal("expected a basic Stat probe after bearer 401")
		}
	})
}

func TestWebDAVClientCodeFlowUsesGuardedTransport(t *testing.T) {
	var davHits, tokenHits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/ocm":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"apiVersion":    "1.2.0",
				"endPoint":      srv.URL + "/ocm",
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case r.URL.Path == "/ocm/token":
			tokenHits++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "jwt-tok",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		case r.Method == "PROPFIND":
			davHits++
			writeReceivedWebDAVPropfind(w, true)
		default:
			http.Error(w, "unexpected path", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	share := testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)
	d := newTestReceivedDriver(t)
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})

	c, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
	if err != nil {
		t.Fatalf("webdavClient returned error: %v", err)
	}
	if tokenHits == 0 {
		t.Fatal("expected a token exchange")
	}
	info, err := c.Stat("")
	if err != nil {
		t.Fatalf("guarded code-flow Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected a collection")
	}
	if davHits == 0 {
		t.Fatal("expected the code-flow WebDAV client to reach the server")
	}
}

func TestWebDAVClientCachesGuardedLegacyClient(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		writeReceivedWebDAVPropfind(w, false)
	}))
	defer srv.Close()

	share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
	d := newTestReceivedDriver(t)
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})

	c1, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
	if err != nil {
		t.Fatalf("first webdavClient returned error: %v", err)
	}
	afterFirst := hits
	if afterFirst == 0 {
		t.Fatal("expected an initial Stat probe")
	}
	c2, _, _, err := d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
	if err != nil {
		t.Fatalf("second webdavClient returned error: %v", err)
	}
	if hits != afterFirst {
		t.Fatalf("cached client probed again: hits %d then %d", afterFirst, hits)
	}
	if c1 != c2 {
		t.Fatal("expected the cached client pointer to be reused")
	}
}

type countingRoundTripper struct {
	rt   http.RoundTripper
	hits int
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.hits++
	return c.rt.RoundTrip(req)
}

func TestWebDAVRetryReconstructsGuardedClient(t *testing.T) {
	var propfinds int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			propfinds++
			writeReceivedWebDAVPropfind(w, false)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
	d := newTestReceivedDriver(t)
	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share, share}})
	guarded := &countingRoundTripper{rt: d.webdavTransport}
	d.webdavTransport = guarded

	ref := &provider.Reference{Path: "/share-abc/docs"}
	fnCalls := 0
	var hitsBeforeRetry int
	err := d.withExchangeRetry(context.Background(), ref, func(_ *gowebdav.Client, _ string) error {
		fnCalls++
		if fnCalls == 1 {
			hitsBeforeRetry = guarded.hits
			return gowebdav.NewPathError("MKCOL", "/docs", http.StatusUnauthorized)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("withExchangeRetry returned error: %v", err)
	}
	if fnCalls != 2 {
		t.Fatalf("expected fn to be called twice, got %d", fnCalls)
	}
	if propfinds == 0 {
		t.Fatal("retry reconstruction must Stat through the guarded client")
	}
	if guarded.hits <= hitsBeforeRetry {
		t.Fatal("retry reconstruction must reuse the public-only transport, not a default client")
	}
}

func TestUploadOnFreshClientUsesGuardedTransport(t *testing.T) {
	t.Run("rejects loopback by default before the server receives a request", func(t *testing.T) {
		var hits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits++
			w.WriteHeader(http.StatusCreated)
		}))
		defer srv.Close()

		d := newReceivedDriver(t, map[string]any{})
		err := d.uploadOnFreshClient(srv.URL, "Bearer tok", "file.txt", bytes.NewReader([]byte("hi")))
		if err == nil {
			t.Fatal("expected upload to refuse loopback")
		}
		if !errors.Is(err, client.ErrPolicyViolation) {
			t.Errorf("uploadOnFreshClient() error = %v, want ErrPolicyViolation", err)
		}
		if hits != 0 {
			t.Fatalf("server hits = %d, want 0", hits)
		}
	})

	t.Run("uploads with allow_loopback_federation", func(t *testing.T) {
		var puts int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				puts++
				w.WriteHeader(http.StatusCreated)
			case "PROPFIND":
				writeReceivedWebDAVPropfind(w, true)
			case "MKCOL":
				w.WriteHeader(http.StatusCreated)
			default:
				w.WriteHeader(http.StatusCreated)
			}
		}))
		defer srv.Close()

		d := newTestReceivedDriver(t)
		err := d.uploadOnFreshClient(srv.URL, "Bearer tok", "file.txt", bytes.NewReader([]byte("hi")))
		if err != nil {
			t.Fatalf("uploadOnFreshClient returned error: %v", err)
		}
		if puts == 0 {
			t.Fatal("expected PUT to reach the server")
		}
	})
}

func TestReceivedExplicitLoopbackAllowsDiscoveryAndWebDAV(t *testing.T) {
	var discoHits, davHits int
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/ocm":
			discoHits++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"enabled":       true,
				"endPoint":      srv.URL + "/ocm",
				"tokenEndPoint": srv.URL + "/ocm/token",
			})
		case r.Method == "PROPFIND":
			davHits++
			writeReceivedWebDAVPropfind(w, false)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newTestReceivedDriver(t)
	share := testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
	_, err := d.getTokenEndpoint(context.Background(), share)
	if err != nil {
		t.Fatalf("getTokenEndpoint returned error: %v", err)
	}
	if discoHits == 0 {
		t.Fatal("expected discovery to reach loopback when opted in")
	}

	stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
	_, _, _, err = d.webdavClient(context.Background(), &provider.Reference{Path: "/share-abc"})
	if err != nil {
		t.Fatalf("webdavClient returned error: %v", err)
	}
	if davHits == 0 {
		t.Fatal("expected WebDAV Stat to reach loopback when opted in")
	}
}

func TestReceivedAllowedFederationCIDRs(t *testing.T) {
	tests := []struct {
		name      string
		cidrs     any
		setKey    bool
		wantErr   bool
		wantParse bool
	}{
		{name: "absent key succeeds", setKey: false},
		{name: "empty list succeeds", cidrs: []any{}, setKey: true},
		{name: "valid ipv4 succeeds", cidrs: []any{"10.197.228.0/24"}, setKey: true},
		{name: "valid ula succeeds", cidrs: []any{"fd42:8c6d:7a10:23::/64"}, setKey: true},
		{name: "valid ipv4 and ula succeeds", cidrs: []any{"10.50.0.0/16", "fd42:8c6d:7a10:23::/64"}, setKey: true},
		{name: "malformed cidr fails", cidrs: []any{"not-a-cidr"}, setKey: true, wantErr: true, wantParse: true},
		{name: "public cidr fails", cidrs: []any{"8.8.8.0/24"}, setKey: true, wantErr: true, wantParse: true},
		{name: "mixed valid invalid fails atomically", cidrs: []any{"10.0.0.0/8", "8.8.8.0/24"}, setKey: true, wantErr: true, wantParse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := map[string]any{}
			if tt.setKey {
				m["allowed_federation_cidrs"] = tt.cidrs
			}
			fs, err := New(context.Background(), m)
			if tt.wantErr {
				if err == nil {
					t.Fatal("New() error = nil, want error")
				}
				if fs != nil {
					t.Fatal("New() returned a driver on error")
				}
				if tt.wantParse && !strings.Contains(err.Error(), "invalid federation CIDR") {
					t.Errorf("error = %v, want it to wrap the parser failure", err)
				}
				if tt.wantParse && !strings.Contains(err.Error(), "allowed_federation_cidrs") {
					t.Errorf("error = %v, want the received config key", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			d, ok := fs.(*driver)
			if !ok || d == nil {
				t.Fatalf("New() type = %T, want *driver", fs)
			}
			if d.ocmClient == nil || d.webdavTransport == nil {
				t.Fatal("New() did not store both received transports")
			}
			tr := client.HTTPTransport(d.webdavTransport)
			if tr == nil {
				t.Fatalf("webdav transport: got %T, want *http.Transport or public-only wrapper", d.webdavTransport)
			}
			if tr.Proxy != nil {
				t.Fatal("default received WebDAV round tripper must not use a proxy")
			}
			if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
				t.Fatal("default received WebDAV round tripper must verify TLS")
			}
			if tr.DialContext == nil {
				t.Fatal("received WebDAV round tripper must install a guarded DialContext")
			}
			if d.c.OCMClientTimeout != 10 {
				t.Errorf("default ocm_timeout = %d, want 10", d.c.OCMClientTimeout)
			}
		})
	}
}

func TestReceivedUseEnvProxyKeepsGuardedDialer(t *testing.T) {
	d := newReceivedDriver(t, map[string]any{
		"ocm_use_env_proxy": true,
	})
	tr := client.HTTPTransport(d.webdavTransport)
	if tr == nil {
		t.Fatalf("webdav transport: got %T, want *http.Transport or public-only wrapper", d.webdavTransport)
	}
	if tr.Proxy == nil {
		t.Fatal("ocm_use_env_proxy true must install the environment proxy")
	}
	if tr.DialContext == nil {
		t.Fatal("ocm_use_env_proxy true must keep the guarded dialer")
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("proxy opt-in must not disable TLS verification")
	}
	if d.c.OCMClientTimeout != 10 {
		t.Errorf("default ocm_timeout = %d, want 10", d.c.OCMClientTimeout)
	}
}

func TestReceivedFederationCIDRPolicyPropagation(t *testing.T) {
	d := newReceivedDriver(t, map[string]any{
		"ocm_timeout": 1,
		"allowed_federation_cidrs": []any{
			"10.50.0.0/16",
			"fd42:8c6d:7a10:23::/64",
		},
	})
	// Mutating the decoded list must not change the policy captured at New.
	d.c.AllowedFederationCIDRs[0] = "192.168.0.0/16"

	for _, addr := range []string{"10.50.1.1:9", "[fd42:8c6d:7a10:23::1]:9"} {
		assertReceivedAdmittedDial(t, dialReceivedTransport(t, d.webdavTransport, addr), addr)
	}
	for _, addr := range []string{"10.9.9.9:9", "192.168.1.1:9", "[fd00::1]:9"} {
		assertReceivedDeniedDial(t, dialReceivedTransport(t, d.webdavTransport, addr), addr)
	}

	inShare := testCodeFlowReceivedShare("10.50.1.1:9", "https://10.50.1.1:9")
	_, err := d.getTokenEndpoint(receivedPolicyCtx(t), inShare)
	assertReceivedAdmitted(t, "discovery in range", err)

	outShare := testCodeFlowReceivedShare("10.9.9.9:9", "https://10.9.9.9:9")
	_, err = d.getTokenEndpoint(receivedPolicyCtx(t), outShare)
	assertReceivedDenied(t, "discovery out of range", err)

	_, err = d.exchangeAccessToken(receivedPolicyCtx(t), inShare, "https://10.50.1.1:9/ocm/token", "secret")
	assertReceivedAdmitted(t, "token in range", err)
	_, err = d.exchangeAccessToken(receivedPolicyCtx(t), outShare, "https://192.168.1.1:9/ocm/token", "secret")
	assertReceivedDenied(t, "token out of range", err)

	inDAV := d.newWebDAVClient("https://10.50.1.1:9/remote.php/dav/ocm/", nil)
	_, err = inDAV.Stat("")
	assertReceivedAdmitted(t, "read in range", err)
	outDAV := d.newWebDAVClient("https://192.168.50.9:9/remote.php/dav/ocm/", nil)
	_, err = outDAV.Stat("")
	assertReceivedDenied(t, "read out of range", err)

	err = d.uploadOnFreshClient("https://10.50.1.1:9/remote.php/dav/ocm/", "Bearer tok", "file.txt", bytes.NewReader([]byte("hi")))
	assertReceivedAdmitted(t, "upload in range", err)
	err = d.uploadOnFreshClient("https://192.168.50.9:9/remote.php/dav/ocm/", "Bearer tok", "file.txt", bytes.NewReader([]byte("hi")))
	assertReceivedDenied(t, "upload out of range", err)

	_, err = d.ocmClient.Discover(receivedPolicyCtx(t), "http://127.0.0.1:9")
	assertReceivedDenied(t, "loopback without allow flag", err)
}

func receivedPolicyCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}

func dialReceivedTransport(t *testing.T, rt http.RoundTripper, address string) error {
	t.Helper()
	tr := client.HTTPTransport(rt)
	if tr == nil {
		t.Fatalf("transport: got %T, want *http.Transport or public-only wrapper", rt)
	}
	if tr.DialContext == nil {
		t.Fatal("received transport must install a guarded DialContext")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}

func assertReceivedAdmittedDial(t *testing.T, err error, addr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("dial %q = nil; want a non-policy net.Error", addr)
	}
	if errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("dial %q = %v; want it to pass the guard", addr, err)
	}
	var netErr net.Error
	if !errors.As(err, &netErr) {
		t.Fatalf("dial %q = %T %v; want a non-policy net.Error", addr, err, err)
	}
}

func assertReceivedDeniedDial(t *testing.T, err error, addr string) {
	t.Helper()
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("dial %q = %v, want ErrPolicyViolation", addr, err)
	}
}

func assertReceivedAdmitted(t *testing.T, op string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil; want a network failure after the guard admits", op)
	}
	if errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("%s = %v; want the guard to admit", op, err)
	}
}

func assertReceivedDenied(t *testing.T, op string, err error) {
	t.Helper()
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("%s = %v, want ErrPolicyViolation", op, err)
	}
}

func TestReceivedCIDRDoesNotRelaxSchemeOrRedirect(t *testing.T) {
	for _, insecure := range []bool{false, true} {
		name := "verify TLS"
		if insecure {
			name = "insecure retains floor"
		}
		t.Run(name, func(t *testing.T) {
			d := newReceivedDriver(t, map[string]any{
				"ocm_timeout":  1,
				"ocm_insecure": insecure,
				"allowed_federation_cidrs": []any{
					"10.50.0.0/16",
					"fd42:8c6d:7a10:23::/64",
				},
			})
			tr := client.HTTPTransport(d.webdavTransport)
			if tr == nil || tr.TLSClientConfig == nil {
				t.Fatal("received WebDAV transport is missing a TLS config")
			}
			if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
				t.Errorf("MinVersion = %v, want TLS 1.2", tr.TLSClientConfig.MinVersion)
			}
			if tr.TLSClientConfig.InsecureSkipVerify != insecure {
				t.Errorf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, insecure)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := d.ocmClient.Discover(ctx, "http://10.50.1.1:9")
			assertReceivedPolicyText(t, "discovery http in range", err, "refusing scheme")
			_, err = d.ocmClient.Discover(ctx, "http://127.0.0.1:9")
			assertReceivedPolicyText(t, "discovery http loopback", err, "refusing scheme")

			_, err = d.newWebDAVClient("http://10.50.1.1:9/", nil).Stat("")
			assertReceivedPolicyText(t, "webdav http in range", err, "refusing scheme")
			ulaReq, err := http.NewRequest(http.MethodGet, "http://[fd42:8c6d:7a10:23::1]/", nil)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.webdavTransport.RoundTrip(ulaReq)
			assertReceivedPolicyText(t, "webdav http ula", err, "refusing scheme")
		})
	}

	d := newReceivedDriver(t, map[string]any{
		"ocm_timeout":               1,
		"ocm_insecure":              true,
		"allow_loopback_federation": true,
		"allowed_federation_cidrs":  []any{"10.50.0.0/16"},
	})

	t.Run("http redirect inside CIDR", func(t *testing.T) {
		var ocmHits, webdavHits int
		tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recordReceivedRedirectHit(t, &ocmHits, &webdavHits, r)
			http.Redirect(w, r, "http://10.50.1.1/ocm", http.StatusFound)
		}))
		defer tlsSrv.Close()
		assertReceivedLiveRedirect(t, d, tlsSrv.URL, "refusing")
		assertReceivedRedirectHits(t, ocmHits, webdavHits)
	})

	t.Run("https redirect outside CIDR", func(t *testing.T) {
		var ocmHits, webdavHits int
		tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recordReceivedRedirectHit(t, &ocmHits, &webdavHits, r)
			http.Redirect(w, r, "https://192.168.9.9:9/ocm", http.StatusFound)
		}))
		defer tlsSrv.Close()
		assertReceivedLiveRedirect(t, d, tlsSrv.URL, "192.168.9.9")
		assertReceivedRedirectHits(t, ocmHits, webdavHits)
	})
}

// recordReceivedRedirectHit splits OCM discovery from WebDAV.
// OCM discovery retries must not be counted as WebDAV hits.
func recordReceivedRedirectHit(t *testing.T, ocmHits, webdavHits *int, r *http.Request) {
	t.Helper()
	switch {
	case r.Method == http.MethodGet && (r.URL.Path == "/.well-known/ocm" || r.URL.Path == "/ocm-provider"):
		*ocmHits++
	case r.Method == "PROPFIND":
		*webdavHits++
	default:
		t.Errorf("unexpected live redirect request %s %s", r.Method, r.URL.Path)
	}
}

func assertReceivedRedirectHits(t *testing.T, ocmHits, webdavHits int) {
	t.Helper()
	// Discover tries /.well-known/ocm, then retries the legacy /ocm-provider endpoint.
	if ocmHits != 2 {
		t.Fatalf("ocm hits = %d, want 2 discovery attempts", ocmHits)
	}
	if webdavHits < 1 {
		t.Fatalf("webdav hits = %d, want at least 1", webdavHits)
	}
}

func assertReceivedPolicyText(t *testing.T, op string, err error, text string) {
	t.Helper()
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Fatalf("%s = %v, want ErrPolicyViolation", op, err)
	}
	if !strings.Contains(err.Error(), text) {
		t.Fatalf("%s = %q, want substring %q", op, err, text)
	}
}

func assertReceivedLiveRedirect(t *testing.T, d *driver, rawURL, text string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := d.ocmClient.Discover(ctx, rawURL)
	assertReceivedPolicyText(t, "ocm redirect", err, text)
	_, err = d.newWebDAVClient(rawURL, nil).Stat("")
	assertReceivedPolicyText(t, "webdav redirect", err, text)
}
