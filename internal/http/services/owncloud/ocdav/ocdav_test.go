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

package ocdav

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/storage/utils/localfs"
	"github.com/cs3org/reva/v3/pkg/utils/resourceid"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

/*
The encodePath method as it is implemented currently is terribly inefficient.
As soon as there are a few special characters which need to be escaped the allocation count rises and the time spent too.
Adding more special characters increases the allocations and the time spent can rise up to a few milliseconds.
Granted this is not a lot on it's own but when a user has tens or hundreds of paths which need to be escaped and contain a few special characters
then this method alone will cost a huge amount of time.
*/
func BenchmarkEncodePath(b *testing.B) {
	for b.Loop() {
		_ = encodePath("/some/path/Folder %^*(#1)")
	}
}

func TestRelativeBasePathFromHeader(t *testing.T) {
	request := httptest.NewRequest(MethodPropfind, "https://example.org/remote.php/dav/spaces/eos-space/project", nil)
	request.Header.Set(HeaderRevaBasePath, "eos/")

	basePath, ok := relativeBasePathFromHeader(request)
	if !ok {
		t.Fatal("expected base path header to be detected")
	}
	if basePath != "/eos" {
		t.Fatalf("base path = %q, want %q", basePath, "/eos")
	}
}

func TestMDToPropResponseUsesRelativeBasePath(t *testing.T) {
	service := svc{c: &Config{}}
	fakeEOSSpaceID := "fake-eos-space-id"

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKeyBaseURI, "/remote.php/dav/spaces")
	ctx = context.WithValue(ctx, ctxSpaceID, fakeEOSSpaceID)
	ctx = context.WithValue(ctx, ctxRelativeBasePath, "/eos")

	res, err := service.mdToPropResponse(ctx, &propfindXML{Allprop: new(struct{})}, &providerv1beta1.ResourceInfo{
		Path: "/eos/project/a/name/public",
		Type: providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
		Id: &providerv1beta1.ResourceId{
			StorageId: "eos-storage-id",
			SpaceId:   spaces.EncodeSpaceID("/eos/project/a/name"),
			OpaqueId:  "node-id",
		},
	}, nil, "", "/remote.php/dav/spaces", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := "/remote.php/dav/spaces/fake-eos-space-id/project/a/name/public/"
	if res.Href != expected {
		t.Fatalf("href = %q, want %q", res.Href, expected)
	}
}

func TestWrapResourceID(t *testing.T) {
	expected := "storageid" + "!" + "opaqueid"
	wrapped := resourceid.OwnCloudResourceIDWrap(&providerv1beta1.ResourceId{StorageId: "storageid", OpaqueId: "opaqueid"})

	if wrapped != expected {
		t.Errorf("wrapped id doesn't have the expected format: got %s expected %s", wrapped, expected)
	}
}

func TestDavFilesPropfindResolvesHomeAndPaths(t *testing.T) {
	endpoint := t.Name()
	service := newLocalFSOCDavService(t, endpoint)

	tests := []struct {
		name      string
		target    string
		depth     string
		wantHrefs []string
	}{
		{
			name:   "home without trailing slash",
			target: "/remote.php/dav/files/user",
			depth:  "1",
			wantHrefs: []string{
				"/remote.php/dav/files/user/",
				"/remote.php/dav/files/user/Documents/",
				"/remote.php/dav/files/user/MyShares/",
				"/remote.php/dav/files/user/root.txt",
			},
		},
		{
			name:   "home with trailing slash",
			target: "/remote.php/dav/files/user/",
			depth:  "1",
			wantHrefs: []string{
				"/remote.php/dav/files/user/",
				"/remote.php/dav/files/user/Documents/",
				"/remote.php/dav/files/user/MyShares/",
				"/remote.php/dav/files/user/root.txt",
			},
		},
		{
			name:   "nested path depth zero",
			target: "/remote.php/dav/files/user/Documents",
			depth:  "0",
			wantHrefs: []string{
				"/remote.php/dav/files/user/Documents/",
			},
		},
		{
			name:   "nested path depth one",
			target: "/remote.php/dav/files/user/Documents",
			depth:  "1",
			wantHrefs: []string{
				"/remote.php/dav/files/user/Documents/",
				"/remote.php/dav/files/user/Documents/report.txt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := newDavPropfindRequest(tt.target, tt.depth)
			response := httptest.NewRecorder()

			service.Handler().ServeHTTP(response, request)

			if response.Code != http.StatusMultiStatus {
				t.Fatalf("expected status %d, got %d: %s", http.StatusMultiStatus, response.Code, response.Body.String())
			}
			assertHrefs(t, response.Body.Bytes(), tt.wantHrefs)
		})
	}
}

func newDavPropfindRequest(target, depth string) *http.Request {
	body := bytes.NewBufferString(`<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><oc:fileid /></d:prop></d:propfind>`)
	request := httptest.NewRequest(MethodPropfind, target, body)
	request.Header.Set(HeaderDepth, depth)
	ctx := context.WithValue(request.Context(), ctxKeyBaseURI, "/remote.php/dav")
	ctx = appctx.ContextSetUser(ctx, &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "user", Idp: "test"},
		Username: "user",
	})
	return request.WithContext(ctx)
}

func newLocalFSOCDavService(t *testing.T, endpoint string) *svc {
	t.Helper()

	ctx := appctx.ContextSetUser(context.Background(), testDavUser())
	fs, err := localfs.NewLocalFS(&localfs.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to create localfs: %v", err)
	}
	t.Cleanup(func() {
		if err := fs.Shutdown(context.Background()); err != nil {
			t.Fatalf("failed to shut down localfs: %v", err)
		}
	})

	if err := fs.CreateHome(ctx); err != nil {
		t.Fatalf("failed to create localfs home: %v", err)
	}
	if err := fs.CreateDir(ctx, &providerv1beta1.Reference{Path: "/Documents"}); err != nil {
		t.Fatalf("failed to create fixture folder: %v", err)
	}
	if err := fs.TouchFile(ctx, &providerv1beta1.Reference{Path: "/Documents/report.txt"}); err != nil {
		t.Fatalf("failed to create fixture nested file: %v", err)
	}
	if err := fs.TouchFile(ctx, &providerv1beta1.Reference{Path: "/root.txt"}); err != nil {
		t.Fatalf("failed to create fixture root file: %v", err)
	}

	gatewayClient := newLocalFSGatewayClient(t, fs)
	pool.RegisterGatewayServiceClient(gatewayClient, endpoint)

	handler := &DavHandler{}
	if err := handler.init(&Config{
		GatewaySvc: endpoint,
	}); err != nil {
		t.Fatalf("failed to init DAV handler: %v", err)
	}
	return &svc{
		c:          &Config{GatewaySvc: endpoint},
		davHandler: handler,
	}
}

func testDavUser() *userv1beta1.User {
	return &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "user", Idp: "test"},
		Username: "user",
	}
}

func newLocalFSGatewayClient(t *testing.T, fs storage.FS) *mockgateway.MockGatewayAPIClient {
	t.Helper()

	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	gatewayClient.EXPECT().GetHome(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, *providerv1beta1.GetHomeRequest, ...grpc.CallOption) (*providerv1beta1.GetHomeResponse, error) {
			return &providerv1beta1.GetHomeResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Path:   "/",
			}, nil
		})
	gatewayClient.EXPECT().Stat(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, req *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
			info, err := fs.GetMD(ctx, req.GetRef(), req.ArbitraryMetadataKeys)
			if err != nil {
				return &providerv1beta1.StatResponse{Status: statusFromLocalFSError(err)}, nil
			}
			return &providerv1beta1.StatResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Info:   info,
			}, nil
		})
	gatewayClient.EXPECT().ListContainer(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, req *providerv1beta1.ListContainerRequest, _ ...grpc.CallOption) (*providerv1beta1.ListContainerResponse, error) {
			infos, err := fs.ListFolder(ctx, req.GetRef(), req.ArbitraryMetadataKeys)
			if err != nil {
				return &providerv1beta1.ListContainerResponse{Status: statusFromLocalFSError(err)}, nil
			}
			return &providerv1beta1.ListContainerResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Infos:  infos,
			}, nil
		})
	mockEmptyShareLookups(gatewayClient)
	return gatewayClient
}

func statusFromLocalFSError(err error) *rpc.Status {
	switch err.(type) {
	case errtypes.IsNotFound:
		return &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: err.Error()}
	case errtypes.PermissionDenied:
		return &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: err.Error()}
	default:
		return &rpc.Status{Code: rpc.Code_CODE_INTERNAL, Message: err.Error()}
	}
}

func assertHrefs(t *testing.T, body []byte, want []string) {
	t.Helper()

	var multistatus struct {
		Responses []struct {
			Href string `xml:"DAV: href"`
		} `xml:"DAV: response"`
	}
	if err := xml.Unmarshal(body, &multistatus); err != nil {
		t.Fatalf("failed to parse PROPFIND response: %v\n%s", err, body)
	}

	got := make([]string, 0, len(multistatus.Responses))
	for _, response := range multistatus.Responses {
		got = append(got, response.Href)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected hrefs\nwant: %v\n got: %v\nbody: %s", want, got, body)
	}
}

func mockEmptyShareLookups(gatewayClient *mockgateway.MockGatewayAPIClient) {
	gatewayClient.On("ListPublicShares", mock.Anything, mock.Anything).Return(&linkv1beta1.ListPublicSharesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil)
	gatewayClient.On("ListShares", mock.Anything, mock.Anything).Return(&collaborationv1beta1.ListSharesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil)
}

func TestPublicFilesSignatureAuthTakesPrecedenceOverBasicAuth(t *testing.T) {
	token := "public-token"
	signature := "signed-value"
	expiration := "2026-06-29T18:48:01+02:00"
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/public-files/public-token/file.txt?oc-signature=signed-value&expiration=2026-06-29T18%3A48%3A01%2B02%3A00", nil)
	request.SetBasicAuth(token, "wrong-password")

	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	gatewayClient.On("Authenticate", mock.Anything, mock.MatchedBy(func(req *gatewayv1beta1.AuthenticateRequest) bool {
		return req.Type == "publicshares" &&
			req.ClientId == token &&
			req.ClientSecret == "signature|"+signature+"|"+expiration
	})).Return(&gatewayv1beta1.AuthenticateResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil).Once()

	res, hasBasicAuthHeader, unauthorized, err := authenticatePublicFilesRequest(context.Background(), request, gatewayClient, token)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if unauthorized {
		t.Fatal("expected signed GET request to be authorized for authentication")
	}
	if !hasBasicAuthHeader {
		t.Fatal("expected request to report the present Basic auth header")
	}
	if res.GetStatus().GetCode() != rpc.Code_CODE_OK {
		t.Fatalf("expected OK status, got %v", res.GetStatus().GetCode())
	}
}

func TestPublicFilesBasicAuthIsUsedWithoutSignature(t *testing.T) {
	token := "public-token"
	password := "public-password"
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/public-files/public-token/file.txt", nil)
	request.SetBasicAuth(token, password)

	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	gatewayClient.On("Authenticate", mock.Anything, mock.MatchedBy(func(req *gatewayv1beta1.AuthenticateRequest) bool {
		return req.Type == "publicshares" &&
			req.ClientId == token &&
			req.ClientSecret == "password|"+password
	})).Return(&gatewayv1beta1.AuthenticateResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil).Once()

	_, hasBasicAuthHeader, unauthorized, err := authenticatePublicFilesRequest(context.Background(), request, gatewayClient, token)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if unauthorized {
		t.Fatal("expected Basic auth request to be authorized for authentication")
	}
	if !hasBasicAuthHeader {
		t.Fatal("expected request to report the present Basic auth header")
	}
}

func TestPublicFilesSignatureAuthRejectsNonGet(t *testing.T) {
	request := httptest.NewRequest(http.MethodDelete, "https://example.org/remote.php/dav/public-files/public-token/file.txt?oc-signature=signed-value&expiration=2026-06-29T18%3A48%3A01%2B02%3A00", nil)

	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	res, _, unauthorized, err := authenticatePublicFilesRequest(context.Background(), request, gatewayClient, "public-token")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !unauthorized {
		t.Fatal("expected signed non-GET request to be unauthorized")
	}
	if res != nil {
		t.Fatalf("expected no authentication response, got %v", res)
	}
}

func TestExtractDestination(t *testing.T) {
	expected := "/dst"
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)
	request.Header.Set(HeaderDestination, "https://example.org/remote.php/dav/dst")

	ctx := context.WithValue(context.Background(), ctxKeyBaseURI, "/remote.php/dav")
	destination, err := extractDestination(request.WithContext(ctx), "")
	if err != nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if destination != expected {
		t.Errorf("Extracted destination is not expected, got %s want %s", destination, expected)
	}
}

func TestExtractDestinationWithoutHeader(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)

	_, err := extractDestination(request, "")
	if err == nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if !errors.Is(err, errInvalidValue) {
		t.Errorf("Expected error invalid value, got %s", err)
	}
}

func TestExtractDestinationWithInvalidDestination(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://example.org/remote.php/dav/src", nil)
	request.Header.Set(HeaderDestination, "://example.org/remote.php/dav/dst")
	_, err := extractDestination(request, "")
	if err == nil {
		t.Errorf("Expected err to be nil got %s", err)
	}

	if !errors.Is(err, errInvalidValue) {
		t.Errorf("Expected error invalid value, got %s", err)
	}
}

func TestNameNotEmptyRule(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		" ":     false,
		"\n":    false,
		"name":  true,
		"empty": true,
	}

	rule := nameNotEmpty{}
	for name, expected := range tests {
		actual := rule.Test(name)
		if actual != expected {
			t.Errorf("For name %s the rule returned %t expected %t", name, actual, expected)
		}
	}
}

func TestNameDoesNotContainRule(t *testing.T) {
	tests := []struct {
		excludedChars string
		tests         map[string]bool
	}{
		{
			"a",
			map[string]bool{
				"foo": true,
				"bar": false,
			},
		},
		{
			"ab",
			map[string]bool{
				"foo": true,
				"bar": false,
				"car": false,
				"bor": false,
			},
		},
	}

	for _, tt := range tests {
		rule := nameDoesNotContain{chars: tt.excludedChars}
		for name, expected := range tt.tests {
			actual := rule.Test(name)
			if actual != expected {
				t.Errorf("For name %s the rule returned %t expected %t", name, actual, expected)
			}
		}
	}
}

func TestIsJWT(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{
			name:  "valid three-segment base64url JWT",
			token: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1In0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			want:  true,
		},
		{
			name:  "UUID legacy secret",
			token: "a3f8c2d1-4b67-11ee-be56-0242ac120002",
			want:  false,
		},
		{
			name:  "two dots but invalid base64url segment",
			token: "abc.!invalid!.xyz",
			want:  false,
		},
		{
			name:  "only two segments",
			token: "abc.def",
			want:  false,
		},
		{
			name:  "empty middle segment",
			token: "abc..def",
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isJWT(tt.token)
			if got != tt.want {
				t.Errorf("isJWT(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestOCMInternalPathLegacyUsesToken(t *testing.T) {
	got, updateIncomingURL := ocmInternalPath("ocmshares", "legacy-token", "share-123", "sub/file.txt")
	if got != "/legacy-token/sub/file.txt" {
		t.Fatalf("ocmInternalPath() = %q, want %q", got, "/legacy-token/sub/file.txt")
	}
	if updateIncomingURL {
		t.Fatal("legacy path should not request ctxKeyIncomingURL update")
	}
}

func TestOCMInternalPathExchangedTokenUsesShareID(t *testing.T) {
	got, updateIncomingURL := ocmInternalPath("ocmexchangedtoken", "jwt-token", "share-123", "sub/file.txt")
	if got != "/share-123/sub/file.txt" {
		t.Fatalf("ocmInternalPath() = %q, want %q", got, "/share-123/sub/file.txt")
	}
	if !updateIncomingURL {
		t.Fatal("exchanged-token path should request ctxKeyIncomingURL update")
	}
}
