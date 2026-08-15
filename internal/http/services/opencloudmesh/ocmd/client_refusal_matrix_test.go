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

package ocmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptrace"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/errtypes"
)

// publicOnlySurfaces are the two constructors that must refuse untrusted
// destinations.
var publicOnlySurfaces = []string{
	"NewPublicOnlyClient",
	"UntrustedHTTPTransport",
}

// TestPublicOnlyClientRefusalMatrix covers the new public-only dial-refusal
// categories. HTTP-scheme and redirect-cap refusals live in client_test.go.
func TestPublicOnlyClientRefusalMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
	}{
		{name: "rfc1918 10/8", rawURL: "https://10.1.2.3/"},
		{name: "rfc1918 172.16/12", rawURL: "https://172.16.5.4/"},
		{name: "rfc1918 192.168/16", rawURL: "https://192.168.1.1/"},
		{name: "cgnat 100.64/10", rawURL: "https://100.64.0.1/"},
		{name: "nat64 well-known prefix", rawURL: "https://[64:ff9b::a9fe:a9fe]/"},
		{name: "link-local 169.254/16", rawURL: "https://169.254.1.1/"},
		{name: "cloud metadata 169.254.169.254", rawURL: "https://169.254.169.254/"},
		{name: "multicast", rawURL: "https://224.0.0.1/"},
		{name: "unspecified", rawURL: "https://0.0.0.0/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, surface := range publicOnlySurfaces {
				t.Run(surface, func(t *testing.T) {
					t.Parallel()
					gotConn, err := doPublicOnlyURL(t, surface, tt.rawURL)
					if gotConn {
						t.Fatalf("%s established a connection to %s", surface, tt.rawURL)
					}
					assertPublicOnlyDialRefusal(t, err)
				})
			}
		})
	}
}

func doPublicOnlyURL(t *testing.T, surface, rawURL string) (bool, error) {
	t.Helper()
	var established bool
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GotConn: func(httptrace.GotConnInfo) {
			established = true
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	var resp *http.Response
	switch surface {
	case "NewPublicOnlyClient":
		resp, err = NewPublicOnlyClient(
			2*time.Second,
			true,
			testSec(),
			0,
		).client.Do(req)
	case "UntrustedHTTPTransport":
		resp, err = (&http.Client{
			Transport: UntrustedHTTPTransport(2*time.Second, true, testSec()),
		}).Do(req)
	default:
		t.Fatalf("unknown public-only surface %q", surface)
	}
	closeResponse(resp)
	return established, err
}

func assertPublicOnlyDialRefusal(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected public-only dial refusal")
	}
	var br errtypes.BadRequest
	if !errors.As(err, &br) {
		t.Fatalf("got %T %v, want errtypes.BadRequest in the chain", err, err)
	}
}
