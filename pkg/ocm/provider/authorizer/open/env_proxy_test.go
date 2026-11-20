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

package open

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

func TestNewUseEnvProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     map[string]any
		wantProxy bool
		wantTLS   bool
		admitCIDR bool
	}{
		{
			name:  "omitted",
			input: map[string]any{},
		},
		{
			name: "false",
			input: map[string]any{
				"ocm_client_use_env_proxy": false,
			},
		},
		{
			name: "true",
			input: map[string]any{
				"ocm_client_use_env_proxy": true,
				"insecure":                 true,
				"allowed_federation_cidrs": []string{"10.1.2.0/24"},
			},
			wantProxy: true,
			wantTLS:   true,
			admitCIDR: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var decoded config
			if err := cfg.Decode(tt.input, &decoded); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded.OCMClientUseEnvProxy != tt.wantProxy {
				t.Fatalf("OCMClientUseEnvProxy = %v, want %v", decoded.OCMClientUseEnvProxy, tt.wantProxy)
			}

			raw, err := New(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			auth, ok := raw.(*authorizer)
			if !ok || auth == nil || auth.ocmClient == nil {
				t.Fatalf("New = %T, want *authorizer with an OCM client", raw)
			}
			tr := client.HTTPTransport(auth.ocmClient.Transport())
			if tr == nil {
				t.Fatalf("transport: got %T, want public-only base *http.Transport", auth.ocmClient.Transport())
			}
			if tr.TLSClientConfig == nil {
				t.Fatal("TLSClientConfig is nil")
			}
			if tr.TLSClientConfig.InsecureSkipVerify != tt.wantTLS {
				t.Fatalf("InsecureSkipVerify = %v, want %v", tr.TLSClientConfig.InsecureSkipVerify, tt.wantTLS)
			}
			if tr.DialContext == nil {
				t.Fatal("DialContext is nil, want the policy dialer")
			}
			if err := dialOpenTransport(tr, "127.0.0.1:9"); !errors.Is(err, client.ErrPolicyViolation) {
				t.Errorf("dial 127.0.0.1:9 = %v, want ErrPolicyViolation", err)
			}
			cidrErr := dialOpenTransport(tr, "10.1.2.3:9")
			if tt.admitCIDR {
				if errors.Is(cidrErr, client.ErrPolicyViolation) {
					t.Errorf("in-range 10.1.2.3 denied by policy = %v, want a non-policy dial error", cidrErr)
				}
			} else if !errors.Is(cidrErr, client.ErrPolicyViolation) {
				t.Errorf("unlisted 10.1.2.3 = %v, want ErrPolicyViolation", cidrErr)
			}
			if !tt.wantProxy {
				if tr.Proxy != nil {
					t.Error("Proxy is set, want nil")
				}
				return
			}
			if tr.Proxy == nil {
				t.Fatal("Proxy is nil, want http.ProxyFromEnvironment")
			}
			got := reflect.ValueOf(tr.Proxy).Pointer()
			want := reflect.ValueOf(http.ProxyFromEnvironment).Pointer()
			if got != want {
				t.Errorf("Proxy function = %#x, want http.ProxyFromEnvironment (%#x)", got, want)
			}
		})
	}
}

func dialOpenTransport(tr *http.Transport, address string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	conn, err := tr.DialContext(ctx, "tcp", address)
	if conn != nil {
		_ = conn.Close()
	}
	return err
}
