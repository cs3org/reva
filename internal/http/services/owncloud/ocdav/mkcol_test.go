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

package ocdav

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

func TestMkcol(t *testing.T) {
	tests := []struct {
		name       string
		statRes    *provider.StatResponse
		statErr    error
		wantFileID string
		wantEtag   string
	}{
		{
			name: "returns fileid and etag of the created collection",
			statRes: &provider.StatResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Info: &provider.ResourceInfo{
					Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER,
					Id:   &provider.ResourceId{StorageId: "storage-id", SpaceId: "space-id", OpaqueId: "opaque-id"},
					Etag: `"deadbeef"`,
				},
			},
			wantFileID: "storage-id$space-id!opaque-id",
			wantEtag:   `"deadbeef"`,
		},
		{
			name:    "returns 201 without headers when the stat status is not ok",
			statRes: &provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_INTERNAL}},
		},
		{
			name:    "returns 201 without headers when the stat call fails",
			statErr: errors.New("transport error"),
		},
	}

	parentRef := &provider.Reference{Path: "/home"}
	childRef := &provider.Reference{Path: "/home/newdir"}
	statOf := func(ref *provider.Reference) interface{} {
		return mock.MatchedBy(func(req *provider.StatRequest) bool { return req.Ref.Path == ref.Path })
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("Stat", mock.Anything, statOf(parentRef)).Return(&provider.StatResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Info:   &provider.ResourceInfo{Type: provider.ResourceType_RESOURCE_TYPE_CONTAINER},
			}, nil).Once()
			gw.On("Stat", mock.Anything, statOf(childRef)).Return(&provider.StatResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND},
			}, nil).Once()
			gw.On("CreateContainer", mock.Anything, mock.Anything).Return(&provider.CreateContainerResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
			}, nil).Once()
			gw.On("Stat", mock.Anything, statOf(childRef)).Return(tt.statRes, tt.statErr).Once()

			endpoint := fmt.Sprintf("mkcol-gw-%d", i)
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := svc{c: &Config{GatewaySvc: endpoint}}

			w := httptest.NewRecorder()
			r := httptest.NewRequest("MKCOL", "http://localhost/home/newdir", nil)
			s.handleMkcol(context.Background(), w, r, parentRef, childRef, zerolog.Logger{})

			if w.Code != http.StatusCreated {
				t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
			}
			if got := w.Header().Get(HeaderOCFileID); got != tt.wantFileID {
				t.Errorf("expected %s header %q, got %q", HeaderOCFileID, tt.wantFileID, got)
			}
			if got := w.Header().Get(HeaderOCETag); got != tt.wantEtag {
				t.Errorf("expected %s header %q, got %q", HeaderOCETag, tt.wantEtag, got)
			}
		})
	}
}
