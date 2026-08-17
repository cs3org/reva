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

package ocm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

// TestUntrustedHardeningRegression is the received-surface T8 sweep. It
// goes through received.New and the constructor-wired WebDAV builders;
// the broad T4/T6 matrices stay in ocm_test.go.
func TestUntrustedHardeningRegression(t *testing.T) {
	t.Run("loopback hatch via received.New", testRegressionReceivedLoopbackHatch)
	t.Run("metadata timeout vs uncapped stream", testRegressionReceivedSplitClients)
}

func testRegressionReceivedLoopbackHatch(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		d := newProductionReceivedDriver(t)
		if d.c.OCMAllowLoopbackFederation || d.sec.AllowLoopback || d.discoverySec.AllowLoopback {
			t.Fatal("ocm_allow_loopback_federation must default off")
		}
		assertReceivedDialRefused(t, receivedPeerDialPolicy(t, d, "https://127.0.0.1:443/"))
		assertReceivedDialRefused(t, receivedDiscoveryDialPolicy(t, d, "https://127.0.0.1:1/"))
	})

	d := newLoopbackHatchReceivedDriver(t)
	if !d.c.OCMAllowLoopbackFederation || !d.sec.AllowLoopback {
		t.Fatal("received.New must enable the WebDAV loopback hatch")
	}
	if d.discoverySec.AllowLoopback {
		t.Fatal("discovery must stay public-only with the hatch on")
	}

	t.Run("webdav hatch allows loopback", func(t *testing.T) {
		assertReceivedDialAllowed(t, receivedPeerDialPolicy(t, d, "https://127.0.0.1:443/"))
		assertReceivedDialAllowed(t, receivedPeerDialPolicy(t, d, "https://[::1]:443/"))

		contacted := false
		davSrv := httptest.NewTLSServer(receivedWebDAVHandler(t, &contacted))
		t.Cleanup(davSrv.Close)
		share := receivedShareWithWebDAVURI(
			davSrv.Listener.Addr().String(),
			"share-abc",
			davSrv.URL+"/remote.php/dav/ocm/share-abc",
		)
		d.gateway = &mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}}
		info, err := d.GetMD(context.Background(), &provider.Reference{Path: "/share-abc"}, nil)
		if err != nil {
			t.Fatalf("GetMD on loopback HTTPS with hatch on: %v", err)
		}
		if info == nil || !contacted {
			t.Fatal("loopback WebDAV must be reachable through received.New")
		}
	})

	t.Run("discovery stays public-only", func(t *testing.T) {
		assertReceivedDialRefused(t, receivedDiscoveryDialPolicy(t, d, "https://127.0.0.1:1/"))

		fetched := false
		disco := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(disco.Close)
		if err := roundTripReceivedOCM(t, d, disco.URL+"/.well-known/ocm"); !errors.Is(err, ocmd.ErrNonPublicAddr) {
			t.Fatalf("discovery Do: got %v, want ocmd.ErrNonPublicAddr", err)
		}
		if fetched {
			t.Fatal("loopback discovery must be refused before contact")
		}
	})

	t.Run("non-loopback private is refused", func(t *testing.T) {
		assertReceivedDialRefused(t, receivedPeerDialPolicy(t, d, "https://10.1.2.3:443/"))
		assertReceivedDialRefused(t, receivedDiscoveryDialPolicy(t, d, "https://10.1.2.3:443/"))
	})
}

func testRegressionReceivedSplitClients(t *testing.T) {
	d := newReceivedDriverWithTimeouts(t, 5, 1, 0)
	if d.c.WebDAVTransferTimeout != 0 {
		t.Fatalf("WebDAVTransferTimeout = %d, want 0", d.c.WebDAVTransferTimeout)
	}

	t.Run("large ReadStream completes with no total cap", func(t *testing.T) {
		const (
			requestTimeout = time.Second
			dripCount      = 8
			dripGap        = 250 * time.Millisecond
		)
		body := bytes.Repeat([]byte("x"), 256*1024)

		t.Run("large ReadStream completes", func(t *testing.T) {
			srv := startPublicTLSServer(t, receivedShareDAVHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))

			stream := d.newReceivedWebDAVStreamClient(srv.URL + "/remote.php/dav/ocm/share-abc")
			rc, err := stream.ReadStream("")
			if err != nil {
				t.Fatalf("ReadStream: %v", err)
			}
			defer rc.Close()
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, body) {
				t.Fatalf("ReadStream truncated: got %d bytes, want %d", len(got), len(body))
			}
		})

		t.Run("slow-drip stream is not truncated", func(t *testing.T) {
			srv := startPublicTLSServer(t, receivedShareDAVHandler(t, nil, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				flusher, _ := w.(http.Flusher)
				for i := 0; i < dripCount; i++ {
					_, _ = w.Write([]byte("x"))
					if flusher != nil {
						flusher.Flush()
					}
					time.Sleep(dripGap)
				}
			}))

			stream := d.newReceivedWebDAVStreamClient(srv.URL + "/remote.php/dav/ocm/share-slow")
			start := time.Now()
			rc, err := stream.ReadStream("")
			if err != nil {
				t.Fatalf("ReadStream: %v", err)
			}
			defer rc.Close()
			got, err := io.ReadAll(rc)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if string(got) != strings.Repeat("x", dripCount) {
				t.Fatalf("slow-drip stream truncated: got %q", got)
			}
			if elapsed < requestTimeout {
				t.Fatalf("elapsed %v did not exceed metadata timeout %v", elapsed, requestTimeout)
			}
			if elapsed < time.Duration(dripCount-1)*dripGap {
				t.Fatalf("expected the drip to take more than webdav_timeout; elapsed %v", elapsed)
			}
		})
	})

	t.Run("metadata client remains bounded", func(t *testing.T) {
		const (
			requestTimeout = time.Second
			dialTimeout    = 5 * time.Second
			peerSleep      = 1200 * time.Millisecond
		)
		started := make(chan struct{})
		srv := startPublicTLSServer(t, receivedShareDAVHandler(t, func(w http.ResponseWriter, r *http.Request) {
			close(started)
			time.Sleep(peerSleep)
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(207)
			_, _ = w.Write([]byte(receivedWebDAVPropfindXML))
		}, nil))

		meta := d.newReceivedWebDAVClient(srv.URL + "/remote.php/dav/ocm/share-abc")
		start := time.Now()
		_, err := meta.Stat("")
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("expected slow Stat to be bounded by webdav_timeout")
		}
		assertReceivedHTTPClientTimeout(t, err)
		if elapsed < requestTimeout/2 {
			t.Fatalf("failed too quickly to be the request deadline: elapsed %v", elapsed)
		}
		if elapsed >= dialTimeout/2 {
			t.Fatalf("elapsed %v looks like the dial deadline, want request timeout %v", elapsed, requestTimeout)
		}
		select {
		case <-started:
		default:
			t.Fatal("expected the slow metadata peer to accept the connection")
		}
	})
}

func assertReceivedHTTPClientTimeout(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected request deadline")
	}
	if strings.Contains(err.Error(), "Client.Timeout") {
		return
	}
	if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	t.Fatalf("got %v, want request deadline, not a dial failure", err)
}
