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
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"testing"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaborationv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
)

// own files come from the home, everybody else's from the files namespace.
// the assertions are on the reference the gateway gets, the hrefs are built
// from the incoming url and look right either way
func TestDavFilesResolvesOwnFilesToHomeAndOthersByName(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantRef string
	}{
		{
			name:    "own home",
			target:  "/remote.php/dav/files/einstein/",
			wantRef: "/home",
		},
		{
			name:    "own nested path",
			target:  "/remote.php/dav/files/einstein/Documents",
			wantRef: "/home/Documents",
		},
		{
			name:    "another user keeps the username in the path",
			target:  "/remote.php/dav/files/marie/Documents",
			wantRef: "/users/marie/Documents",
		},
		{
			name:    "the username is matched case insensitively",
			target:  "/remote.php/dav/files/Einstein/Documents",
			wantRef: "/home/Documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var statted []string
			service := newRecordingOCDavService(t, &statted, nil)

			response := httptest.NewRecorder()
			service.Handler().ServeHTTP(response, newFilesRequest(MethodPropfind, tt.target, nil))

			if response.Code != http.StatusMultiStatus {
				t.Fatalf("expected status %d, got %d: %s", http.StatusMultiStatus, response.Code, response.Body.String())
			}
			if !slices.Contains(statted, tt.wantRef) {
				t.Fatalf("expected a stat on %q, got %v", tt.wantRef, statted)
			}
		})
	}
}

// the destination header is resolved through the namespace and not through the
// request path, so source and destination have to end up under the same prefix
func TestDavFilesMoveResolvesDestinationInTheSameNamespace(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		destination string
		wantSrc     string
		wantDst     string
	}{
		{
			name:        "own files move within the home",
			source:      "/remote.php/dav/files/einstein/Documents",
			destination: "http://localhost/remote.php/dav/files/einstein/Renamed",
			wantSrc:     "/home/Documents",
			wantDst:     "/home/Renamed",
		},
		{
			name:        "another user's files move within the files namespace",
			source:      "/remote.php/dav/files/marie/Documents",
			destination: "http://localhost/remote.php/dav/files/marie/Renamed",
			wantSrc:     "/users/marie/Documents",
			wantDst:     "/users/marie/Renamed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var statted []string
			moved := make(chan [2]string, 1)
			service := newRecordingOCDavService(t, &statted, moved)

			request := newFilesRequest(MethodMove, tt.source, nil)
			request.Header.Set(HeaderDestination, tt.destination)

			response := httptest.NewRecorder()
			service.Handler().ServeHTTP(response, request)

			if response.Code != http.StatusCreated {
				t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, response.Code, response.Body.String())
			}
			select {
			case got := <-moved:
				if got[0] != tt.wantSrc || got[1] != tt.wantDst {
					t.Fatalf("moved %q -> %q, want %q -> %q", got[0], got[1], tt.wantSrc, tt.wantDst)
				}
			default:
				t.Fatalf("no move was issued, stats were %v", statted)
			}
		})
	}
}

// clients send OPTIONS before they log in, so it must not need a user
func TestDavFilesOptionsNeedsNoUserAndNoGateway(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{
			name:   "own home",
			target: "/remote.php/dav/files/einstein/",
		},
		{
			name:   "own nested path",
			target: "/remote.php/dav/files/einstein/Documents",
		},
		{
			name:   "another user",
			target: "/remote.php/dav/files/marie/Documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := t.Name()
			// no expectations, so any call to the gateway fails the test
			pool.RegisterGatewayServiceClient(mockgateway.NewMockGatewayAPIClient(t), endpoint)

			handler := &DavHandler{}
			if err := handler.init(&Config{GatewaySvc: endpoint, FilesNamespace: "/users"}); err != nil {
				t.Fatalf("failed to init DAV handler: %v", err)
			}
			service := &svc{c: &Config{GatewaySvc: endpoint}, davHandler: handler}

			response := httptest.NewRecorder()
			service.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodOptions, tt.target, nil))

			if response.Code != http.StatusNoContent {
				t.Fatalf("expected status %d, got %d: %s", http.StatusNoContent, response.Code, response.Body.String())
			}
			if response.Header().Get("Allow") == "" {
				t.Fatal("expected an Allow header")
			}
		})
	}
}

func newFilesRequest(method, target string, body []byte) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set(HeaderDepth, "0")
	ctx := appctx.ContextSetUser(request.Context(), &userv1beta1.User{
		Id:       &userv1beta1.UserId{OpaqueId: "einstein-id", Idp: "test"},
		Username: "einstein",
	})
	return request.WithContext(ctx)
}

// records the paths the gateway is asked to stat, and the move it is asked to
// perform when moved is not nil
func newRecordingOCDavService(t *testing.T, statted *[]string, moved chan [2]string) *svc {
	t.Helper()

	endpoint := t.Name()
	movedYet := false
	client := mockgateway.NewMockGatewayAPIClient(t)
	client.EXPECT().GetHome(mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, *providerv1beta1.GetHomeRequest, ...grpc.CallOption) (*providerv1beta1.GetHomeResponse, error) {
			return &providerv1beta1.GetHomeResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Path:   "/home",
			}, nil
		}).Maybe()
	client.EXPECT().Stat(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req *providerv1beta1.StatRequest, _ ...grpc.CallOption) (*providerv1beta1.StatResponse, error) {
			ref := req.GetRef().GetPath()
			*statted = append(*statted, ref)
			// the destination of a move is free until the move has run
			if moved != nil && !movedYet && path.Base(ref) == "Renamed" {
				return &providerv1beta1.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil
			}
			return &providerv1beta1.StatResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Info: &providerv1beta1.ResourceInfo{
					Id:   &providerv1beta1.ResourceId{StorageId: "storage", OpaqueId: "opaque"},
					Path: ref,
					Type: providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER,
				},
			}, nil
		})
	client.EXPECT().Move(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req *providerv1beta1.MoveRequest, _ ...grpc.CallOption) (*providerv1beta1.MoveResponse, error) {
			movedYet = true
			moved <- [2]string{req.GetSource().GetPath(), req.GetDestination().GetPath()}
			return &providerv1beta1.MoveResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}}, nil
		}).Maybe()
	// only a propfind looks these up
	client.On("ListPublicShares", mock.Anything, mock.Anything).Return(&linkv1beta1.ListPublicSharesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil).Maybe()
	client.On("ListShares", mock.Anything, mock.Anything).Return(&collaborationv1beta1.ListSharesResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
	}, nil).Maybe()
	pool.RegisterGatewayServiceClient(client, endpoint)

	handler := &DavHandler{}
	if err := handler.init(&Config{GatewaySvc: endpoint, FilesNamespace: "/users"}); err != nil {
		t.Fatalf("failed to init DAV handler: %v", err)
	}
	return &svc{c: &Config{GatewaySvc: endpoint}, davHandler: handler}
}
