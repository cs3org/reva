// Copyright 2018-2019 CERN
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

package rhttp

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/cs3org/reva/pkg/token"
	"github.com/pkg/errors"
	"go.opencensus.io/plugin/ochttp"
)

const (
	defaultAccessHeader = "X-Access-Token"
)

// GetHTTPClient returns an http client with open census tracing support.
// TODO(labkode): harden it.
// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func GetHTTPClient(ctx context.Context) *http.Client {
	httpClient := &http.Client{
		Timeout:   time.Second * 10,
		Transport: &ochttp.Transport{},
	}
	return httpClient
}

// NewRequest creates an HTTP request that sets the token if it is passed in ctx.
func NewRequest(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	httpReq, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "utils: error creating request")
	}

	// TODO(labkode): make header / auth configurable
	tkn, ok := token.ContextGetToken(ctx)
	if ok {
		httpReq.Header.Set(defaultAccessHeader, tkn)
	}

	httpReq = httpReq.WithContext(ctx)
	return httpReq, nil
}
