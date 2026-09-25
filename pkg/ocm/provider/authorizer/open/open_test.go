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

package open_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/provider/authorizer/open"
)

func TestGetInfoByDomainRejectsLoopbackBeforeRequest(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		t.Error("loopback target received a request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	auth, err := open.New(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = auth.GetInfoByDomain(ctx, srv.URL)
	if err == nil {
		t.Fatal("GetInfoByDomain: expected loopback domain to be rejected")
	}
	if hits.Load() != 0 {
		t.Fatalf("loopback server received %d requests, want 0", hits.Load())
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("GetInfoByDomain error = %v, want errors.Is ErrPolicyViolation", err)
	}
	if !strings.Contains(err.Error(), "error probing OCM services at remote server") {
		t.Errorf("GetInfoByDomain error = %v, want wrapped probing error", err)
	}
}

func TestGetInfoByDomainRejectsRFC1918Literal(t *testing.T) {
	t.Parallel()

	auth, err := open.New(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = auth.GetInfoByDomain(ctx, "10.1.2.3")
	if err == nil {
		t.Fatal("GetInfoByDomain: expected RFC1918 literal to be rejected")
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("GetInfoByDomain error = %v, want errors.Is ErrPolicyViolation", err)
	}
	if !strings.Contains(err.Error(), "error probing OCM services at remote server") {
		t.Errorf("GetInfoByDomain error = %v, want wrapped probing error", err)
	}
}

func TestGetInfoByDomainMalformedDomainWrappedError(t *testing.T) {
	t.Parallel()

	auth, err := open.New(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = auth.GetInfoByDomain(ctx, "http://[")
	if err == nil {
		t.Fatal("GetInfoByDomain: expected malformed domain to return an error")
	}
	if !strings.Contains(err.Error(), "error probing OCM services at remote server") {
		t.Errorf("GetInfoByDomain error = %v, want wrapped probing error", err)
	}
	cause := errors.Unwrap(err)
	if cause == nil {
		t.Fatal("GetInfoByDomain error was not wrapped")
	}
	if strings.TrimSpace(cause.Error()) == "" {
		t.Fatal("wrapped cause is empty")
	}
}

func TestNewRejectsInvalidFederationCIDR(t *testing.T) {
	t.Parallel()

	// The shared parser's sentinel is unexported; callers identify it by its
	// fixed message text. A decode error would not contain this text.
	const invalidCIDR = "invalid federation CIDR"

	tests := []struct {
		name   string
		values []string
	}{
		{name: "invalid after valid", values: []string{"10.0.0.0/8", "nope"}},
		{name: "public range", values: []string{"8.8.8.8/32"}},
		{name: "host bits", values: []string{"10.1.2.3/8"}},
		{name: "loopback", values: []string{"127.0.0.0/8"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := open.New(context.Background(), map[string]any{
				"allowed_federation_cidrs": tt.values,
			})
			if err == nil {
				t.Fatalf("New: expected error for %v, got nil", tt.values)
			}
			if !strings.Contains(err.Error(), invalidCIDR) {
				t.Fatalf("New error = %v, want %q in message", err, invalidCIDR)
			}
		})
	}
}

func TestNewRejectsScalarFederationCIDRs(t *testing.T) {
	t.Parallel()

	_, err := open.New(context.Background(), map[string]any{
		"allowed_federation_cidrs": "10.0.0.0/8",
	})
	if err == nil {
		t.Fatal("New: expected decode error for scalar allowed_federation_cidrs")
	}
	if strings.Contains(err.Error(), "invalid federation CIDR") {
		t.Fatalf("scalar decode error = %v, must not be the CIDR parse error", err)
	}
}

func TestNewRejectsMixedValueFederationCIDRs(t *testing.T) {
	t.Parallel()

	_, err := open.New(context.Background(), map[string]any{
		"allowed_federation_cidrs": []any{"10.0.0.0/8", 42},
	})
	if err == nil {
		t.Fatal("New: expected decode error for mixed-value allowed_federation_cidrs")
	}
	if strings.Contains(err.Error(), "invalid federation CIDR") {
		t.Fatalf("mixed-value decode error = %v, must not be the CIDR parse error", err)
	}
}

func TestNewAcceptsValidFederationCIDRs(t *testing.T) {
	t.Parallel()

	auth, err := open.New(context.Background(), map[string]any{
		"allowed_federation_cidrs": []string{"10.1.2.0/24", "fd12:3456:789a::/48"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if auth == nil {
		t.Fatal("New returned nil authorizer")
	}
}

func TestGetInfoByDomainDeniesUnlistedRFC1918WithConfiguredRange(t *testing.T) {
	t.Parallel()

	// Configure an explicit 10.1.2.0/24 exception. A different RFC1918 range
	// stays denied: the configured exception is not a broad private allowlist.
	auth, err := open.New(context.Background(), map[string]any{
		"allowed_federation_cidrs": []string{"10.1.2.0/24"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = auth.GetInfoByDomain(ctx, "192.168.1.50")
	if err == nil {
		t.Fatal("GetInfoByDomain: expected unlisted RFC1918 literal to be rejected")
	}
	if !errors.Is(err, client.ErrPolicyViolation) {
		t.Errorf("GetInfoByDomain error = %v, want errors.Is ErrPolicyViolation", err)
	}
	if !strings.Contains(err.Error(), "error probing OCM services at remote server") {
		t.Errorf("GetInfoByDomain error = %v, want wrapped probing error", err)
	}
}

func TestGetInfoByDomainAdmitsConfiguredRangeBeforeDial(t *testing.T) {
	t.Parallel()

	// Configure 10.1.2.0/24 as an explicit exception. A literal in that range
	// passes the public-only address guard. A completed dial is admitted
	// discovery. A dial that fails must be a network error, not a policy
	// violation. This exercises the actual constructor wiring without
	// duplicating H1's normalization rules.
	auth, err := open.New(context.Background(), map[string]any{
		"allowed_federation_cidrs": []string{"10.1.2.0/24"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	info, err := auth.GetInfoByDomain(ctx, "10.1.2.3")
	if err != nil {
		if errors.Is(err, client.ErrPolicyViolation) {
			t.Fatalf("GetInfoByDomain: in-range 10.1.2.3 denied by policy = %v, want a non-policy dial error", err)
		}
		if !strings.Contains(err.Error(), "error probing OCM services at remote server") {
			t.Errorf("GetInfoByDomain error = %v, want wrapped probing error", err)
		}
		return
	}
	if info == nil {
		t.Fatal("GetInfoByDomain: admitted 10.1.2.3 returned a nil provider")
	}
	if info.Domain != "10.1.2.3" {
		t.Fatalf("GetInfoByDomain domain = %q, want admitted 10.1.2.3", info.Domain)
	}
}
