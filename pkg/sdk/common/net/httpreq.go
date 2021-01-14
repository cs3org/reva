// Copyright 2018-2021 CERN
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

package net

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

// HTTPRequest performs Reva-specific requests through an HTTP endpoint.
type HTTPRequest struct {
	endpoint string
	data     io.Reader

	client  *http.Client
	request *http.Request
}

func (request *HTTPRequest) initRequest(ctx context.Context, endpoint string, method string, accessToken string, transportToken string, data io.Reader) error {
	request.endpoint = endpoint
	request.data = data

	// Initialize the HTTP client
	request.client = &http.Client{
		Timeout: time.Duration(24 * int64(time.Hour)),
	}

	// Initialize the HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, endpoint, data)
	if err != nil {
		return fmt.Errorf("unable to create the HTTP request: %v", err)
	}
	request.request = httpReq

	// Set mandatory header values
	request.request.Header.Set(AccessTokenName, accessToken)
	request.request.Header.Set(TransportTokenName, transportToken)

	return nil
}

func (request *HTTPRequest) do() (*http.Response, error) {
	httpRes, err := request.client.Do(request.request)
	if err != nil {
		return nil, fmt.Errorf("unable to do the HTTP request: %v", err)
	}
	if httpRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("performing the HTTP request failed: %v", httpRes.Status)
	}
	return httpRes, nil
}

// AddParameters adds the specified parameters to the request.
// The parameters are passed in the query URL.
func (request *HTTPRequest) AddParameters(params map[string]string) {
	query := request.request.URL.Query()
	for k, v := range params {
		query.Add(k, v)
	}
	request.request.URL.RawQuery = query.Encode()
}

// Do performs the request on the HTTP endpoint and returns the body data.
// If checkStatus is set to true, the call will only succeed if the server returns a status code of 200.
func (request *HTTPRequest) Do(checkStatus bool) ([]byte, error) {
	httpRes, err := request.do()
	if err != nil {
		return nil, fmt.Errorf("unable to perform the HTTP request for '%v': %v", request.endpoint, err)
	}
	defer httpRes.Body.Close()

	if checkStatus && httpRes.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received invalid response from '%v': %s", request.endpoint, httpRes.Status)
	}

	data, err := ioutil.ReadAll(httpRes.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response data from '%v' failed: %v", request.endpoint, err)
	}
	return data, nil
}

// NewHTTPRequest creates a new HTTP request.
func NewHTTPRequest(ctx context.Context, endpoint string, method string, accessToken string, transportToken string, data io.Reader) (*HTTPRequest, error) {
	request := &HTTPRequest{}
	if err := request.initRequest(ctx, endpoint, method, accessToken, transportToken, data); err != nil {
		return nil, fmt.Errorf("unable to initialize the HTTP request: %v", err)
	}
	return request, nil
}
