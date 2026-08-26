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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/httpclient"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

// Test that when calls come in to the PUT endpoint with a X-Disable-Versioning header,
// this header is propagated to the actual upload endpoint
func TestDisableVersioningHeaderPassedAlong(t *testing.T) {

	gatewayAPIEndpoint := "my-api-endpoint"
	incomingPath := "http://my-reva.com/myfile.txt"
	input := "Hello world!"

	// create HTTP request
	request := httptest.NewRequest(http.MethodPut, incomingPath, strings.NewReader(input))
	request.Header.Add(HeaderContentLength, strconv.Itoa(len(input)))
	request.Header.Add(HeaderDisableVersioning, "true")

	// Create fake HTTP server for upload endpoint
	// Here we will check whether the header was correctly set
	calls := 0
	w := httptest.NewRecorder()
	mockServerUpload := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				if header := r.Header.Get(HeaderDisableVersioning); header == "" {
					t.Errorf("expected header %s but header was not set", HeaderDisableVersioning)
				}
				calls++
			},
		),
	)
	endpointPath := mockServerUpload.URL

	// Set up mocked GatewayAPIClient
	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{Status: &rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}}, nil)
	gatewayClient.On("InitiateFileUpload", mock.Anything, mock.Anything).Return(&gateway.InitiateFileUploadResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Protocols: []*gateway.FileUploadProtocol{
			{Protocol: "simple", UploadEndpoint: endpointPath, Token: "my-secret-token"},
		}}, nil)
	stampGateway(gatewayClient)

	// Set up OCDAV Service
	service := svc{
		c: &Config{
			GatewaySvc: gatewayAPIEndpoint,
		},
		client: httpclient.New(),
	}
	ref := provider.Reference{}

	// Do the actual call
	service.handlePut(context.Background(), w, request, &ref, zerolog.Logger{})

	// If no connection was made to the upload endpoint, something is also wrong
	if calls == 0 {
		t.Errorf("Upload endpoint was not called. ")
	}
}

// Test that an upload by a user with the uploader role (e.g. through an upload-only
// public link), which gets stored under a randomized name, reports that name back
// to the client
func TestUploaderRolePutReturnsFilename(t *testing.T) {

	gatewayAPIEndpoint := "uploader-api-endpoint"
	incomingPath := "http://my-reva.com/myfile.txt"
	input := "Hello world!"

	request := httptest.NewRequest(http.MethodPut, incomingPath, strings.NewReader(input))
	request.Header.Add(HeaderContentLength, strconv.Itoa(len(input)))

	w := httptest.NewRecorder()
	mockServerUpload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer mockServerUpload.Close()

	// The path is only randomized after the initial stat, so we record the name
	// the upload was actually initiated for
	var uploadedPath string

	gatewayClient := mockgateway.NewMockGatewayAPIClient(t)
	// the file does not exist yet, and the stat after the upload returns the new file
	gatewayClient.On("Stat", mock.Anything, mock.Anything).Return(&provider.StatResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Info: &provider.ResourceInfo{
			Type:  provider.ResourceType_RESOURCE_TYPE_FILE,
			Id:    &provider.ResourceId{StorageId: "storage-id", OpaqueId: "opaque-id"},
			Mtime: &typespb.Timestamp{Seconds: 1},
		},
	}, nil)
	gatewayClient.On("InitiateFileUpload", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		uploadedPath = args.Get(1).(*provider.InitiateFileUploadRequest).Ref.Path
	}).Return(&gateway.InitiateFileUploadResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Protocols: []*gateway.FileUploadProtocol{
			{Protocol: "simple", UploadEndpoint: mockServerUpload.URL, Token: "my-secret-token"},
		}}, nil)
	stampGateway(gatewayClient)

	service := svc{
		c: &Config{
			GatewaySvc: gatewayAPIEndpoint,
		},
		client: httpclient.New(),
	}
	ref := provider.Reference{Path: "/myfile.txt"}

	// a public link user with the uploader role
	ctx := appctx.ContextSetUser(context.Background(), &userpb.User{
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"public-share-role": {Decoder: "plain", Value: []byte("uploader")},
			},
		},
	})

	service.handlePut(ctx, w, request, &ref, zerolog.Logger{})

	res := w.Result()
	defer res.Body.Close()

	// the upload created a new file, so it must be a 201: a 204 cannot carry content
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, res.StatusCode)
	}
	if contentType := res.Header.Get(HeaderContentType); contentType != "application/json" {
		t.Errorf("expected content type application/json, got %s", contentType)
	}

	var body putResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("error decoding response body: %s", err)
	}
	if expected := path.Base(uploadedPath); body.Filename != expected {
		t.Errorf("expected filename %s, got %s", expected, body.Filename)
	}
	// the name must be the randomized one, not the one the client asked for
	if body.Filename == "myfile.txt" {
		t.Errorf("expected a randomized filename, got %s", body.Filename)
	}
}
