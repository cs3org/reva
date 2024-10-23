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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	mockgateway "github.com/cs3org/go-cs3apis/mocks/github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/pkg/httpclient"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
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
	pool.RegisterGatewayServiceClient(gatewayClient, gatewayAPIEndpoint)

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
