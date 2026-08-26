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

package embedded

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

// TestUntrustedHardeningRegression proves embedded.New wires the untrusted
// src client used by transferEntries/uploadURLToWebDAV. Per-host matrices
// stay in embedded_test.go.
func TestUntrustedHardeningRegression(t *testing.T) {
	dest := startDestWebDAV(t)
	got, err := New(context.Background(), map[string]any{
		"webdav_url":                     dest.URL,
		"embedded_transfer_retries":      1,
		"embedded_transfer_timeout":      5,
		"embedded_transfer_idle_timeout": 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := got.(*driver)
	if !ok {
		t.Fatalf("got %T, want *driver", got)
	}

	t.Run("HTTP is refused", func(t *testing.T) {
		fetched := false
		src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("secret"))
		}))
		t.Cleanup(src.Close)

		err := d.transferEntries(
			testLogger(),
			"tok",
			"/dest",
			[]transferEntry{{
				srcURL:   src.URL + "/file.txt",
				name:     "file.txt",
				sizeHint: -1,
			}},
			5*time.Second,
		)
		if fetched {
			t.Fatal("embedded.New transferEntries must not fetch HTTP")
		}
		if err != nil {
			t.Fatalf("transferEntries returned %v, want nil (per-file failures are skipped)", err)
		}
	})

	t.Run("metadata host is refused", func(t *testing.T) {
		err := d.transferEntries(
			testLogger(),
			"tok",
			"/dest",
			[]transferEntry{{
				srcURL:   "https://169.254.169.254/file.txt",
				name:     "file.txt",
				sizeHint: -1,
			}},
			5*time.Second,
		)
		if err != nil {
			t.Fatalf("transferEntries returned %v, want nil (per-file failures are skipped)", err)
		}

		// transferEntries swallows the src error. uploadURLToWebDAV is the
		// GET it uses, so the sentinel is visible here.
		ctx, tr := contextWithHostDialTrace(context.Background())
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		err = uploadURLToWebDAV(
			ctx,
			testLogger(),
			d.productionSrcClient(),
			dummyDAV(),
			"https://169.254.169.254/file.txt",
			"/dest/file.txt",
			-1,
			2*time.Second,
		)
		assertNoEstablishedHost(t, tr, "169.254.169.254")
		if !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("got %v, want ocmd.ErrNonPublicAddr", err)
		}
	})

	t.Run("public HTTPS is not blocked as non-public", func(t *testing.T) {
		contacted := false
		src := startPublicTLSServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			contacted = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("embedded-src"))
		}))

		err := d.transferEntries(
			testLogger(),
			"tok",
			"/dest",
			[]transferEntry{{
				srcURL:   src.URL + "/file.txt",
				name:     "file.txt",
				sizeHint: -1,
			}},
			5*time.Second,
		)
		if err != nil {
			t.Fatalf("transferEntries returned %v, want nil (per-file failures are skipped)", err)
		}
		if contacted {
			t.Fatal("production src client verifies TLS; httptest cert must fail after the public dial")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = uploadURLToWebDAV(
			ctx,
			testLogger(),
			d.productionSrcClient(),
			dummyDAV(),
			src.URL+"/file.txt",
			"/dest/file.txt",
			-1,
			2*time.Second,
		)
		if errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatal("public HTTPS must not be refused as non-public")
		}
		if err == nil || contacted {
			t.Fatal("production src client verifies TLS; httptest cert must fail after the public dial")
		}
	})
}

// productionSrcClient is the untrusted client transferEntries builds after New:
// insecure is false, and timeout/security/TLS come from the constructor.
func (d *driver) productionSrcClient() *http.Client {
	return newEmbeddedSrcClient(d.srcDialTimeout(), false, d.sec, d.tlsMinVersion)
}
