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
	"net/http"
	"net/http/httptest"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/stretchr/testify/mock"
)

// The caller writes "success: Created" whenever this returns nil, and the 202 has
// already gone out, so that trailer is all the client sees.
func TestPerformHTTPPullPropagatesUploadFailure(t *testing.T) {
	tests := []struct {
		name             string
		uploadStatusCode int
		wantErr          bool
	}{
		{name: "upload succeeds", uploadStatusCode: http.StatusOK, wantErr: false},
		{name: "upload fails", uploadStatusCode: http.StatusInternalServerError, wantErr: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello"))
			}))
			defer source.Close()

			upload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.uploadStatusCode)
			}))
			defer upload.Close()

			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("InitiateFileUpload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileUploadResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Protocols: []*gateway.FileUploadProtocol{
					{Protocol: "simple", UploadEndpoint: upload.URL, Token: "upload-token"},
				},
			}, nil)

			endpoint := "tpc-pull-gw-" + string(rune('a'+i))
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := svc{c: &Config{GatewaySvc: endpoint}, client: httpclient.New()}

			r := httptest.NewRequest("COPY", "http://localhost/dst.txt", nil)
			r.Header.Set("Source", source.URL)

			err := s.performHTTPPull(context.Background(), gw, r, httptest.NewRecorder(), "/ns")

			if (err != nil) != tt.wantErr {
				t.Fatalf("performHTTPPull() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Same for the push direction, where the PUT goes to the remote destination.
func TestPerformHTTPPushPropagatesUploadFailure(t *testing.T) {
	tests := []struct {
		name             string
		uploadStatusCode int
		wantErr          bool
	}{
		{name: "upload succeeds", uploadStatusCode: http.StatusOK, wantErr: false},
		{name: "upload fails", uploadStatusCode: http.StatusInternalServerError, wantErr: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("hello"))
			}))
			defer download.Close()

			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.uploadStatusCode)
			}))
			defer destination.Close()

			gw := mockgateway.NewMockGatewayAPIClient(t)
			gw.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
				Status: &rpc.Status{Code: rpc.Code_CODE_OK},
				Protocols: []*gateway.FileDownloadProtocol{
					{Protocol: "simple", DownloadEndpoint: download.URL, Token: "download-token"},
				},
			}, nil)

			endpoint := "tpc-push-gw-" + string(rune('a'+i))
			pool.RegisterGatewayServiceClient(gw, endpoint)
			s := svc{c: &Config{GatewaySvc: endpoint}, client: httpclient.New()}

			r := httptest.NewRequest("COPY", "http://localhost/src.txt", nil)
			r.Header.Set("Destination", destination.URL)

			srcInfo := &provider.ResourceInfo{
				Type: provider.ResourceType_RESOURCE_TYPE_FILE,
				Path: "/src.txt",
				Size: 5,
			}

			err := s.performHTTPPush(context.Background(), gw, r, httptest.NewRecorder(), srcInfo, "/ns")

			if (err != nil) != tt.wantErr {
				t.Fatalf("performHTTPPush() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
