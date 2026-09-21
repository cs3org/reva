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
