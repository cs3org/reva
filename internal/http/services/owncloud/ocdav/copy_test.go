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
	"github.com/stretchr/testify/mock"
)

func TestCopyFailsWhenUploadFails(t *testing.T) {
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer download.Close()
	upload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upload.Close()

	gw := mockgateway.NewMockGatewayAPIClient(t)
	gw.On("InitiateFileDownload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileDownloadResponse{
		Status:    &rpc.Status{Code: rpc.Code_CODE_OK},
		Protocols: []*gateway.FileDownloadProtocol{{Protocol: "simple", DownloadEndpoint: download.URL}},
	}, nil)
	gw.On("InitiateFileUpload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileUploadResponse{
		Status:    &rpc.Status{Code: rpc.Code_CODE_OK},
		Protocols: []*gateway.FileUploadProtocol{{Protocol: "simple", UploadEndpoint: upload.URL}},
	}, nil)
	s := svc{c: &Config{}, client: httpclient.New()}

	cp := &copy{
		sourceInfo:  &provider.ResourceInfo{Type: provider.ResourceType_RESOURCE_TYPE_FILE, Path: "/src.txt"},
		destination: &provider.Reference{Path: "/dst.txt"},
		depth:       "0",
	}
	r := httptest.NewRequest("COPY", "/dst.txt", nil)
	if err := s.executePathCopy(context.Background(), gw, httptest.NewRecorder(), r, cp); err == nil {
		t.Fatal("expected an error when the upload fails")
	}
}
