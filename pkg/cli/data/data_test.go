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

package data

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appprovider "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

type locator struct {
	url       string
	checksums []*provider.ResourceChecksumPriority
}

func (l locator) DownloadTarget(context.Context, *provider.Reference) (*backend.Target, error) {
	return &backend.Target{URL: l.url, Header: http.Header{"X-Reva-Transfer": {"tx"}}}, nil
}

func (l locator) UploadTarget(_ context.Context, _ *provider.Reference, _ int64, ifMatch string) (*backend.Target, error) {
	h := http.Header{"X-Reva-Transfer": {"tx"}}
	if ifMatch != "" {
		h.Set("If-Match", ifMatch)
	}
	return &backend.Target{URL: l.url, Header: h, Checksums: l.checksums}, nil
}

func (locator) OpenInApp(context.Context, *provider.Reference, backend.AppOptions) (*appprovider.OpenInAppURL, error) {
	return nil, nil
}

func TestDownloadOffset(t *testing.T) {
	for _, honorRange := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Reva-Transfer") != "tx" {
				http.Error(w, "no token", http.StatusUnauthorized)
				return
			}
			if !honorRange {
				r.Header.Del("Range")
			}
			http.ServeContent(w, r, "f", time.Time{}, strings.NewReader("0123456789"))
		}))
		p := New(srv.Client(), locator{url: srv.URL})
		body, size, err := p.Download(context.Background(), &provider.Reference{Path: "/f"}, 4)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(body)
		body.Close()
		if string(got) != "456789" || size != 6 {
			t.Errorf("range honored %v: got %q, size %d; want 456789, 6", honorRange, got, size)
		}
		srv.Close()
	}
}

func TestUpload(t *testing.T) {
	var query, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Match") == "old" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		b, _ := io.ReadAll(r.Body)
		query, body = r.URL.RawQuery, string(b)
	}))
	defer srv.Close()
	loc := locator{url: srv.URL, checksums: []*provider.ResourceChecksumPriority{
		{Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5, Priority: 2},
		{Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32, Priority: 1},
	}}
	p := New(srv.Client(), loc)
	open := func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("hello")), nil }
	ref := &provider.Reference{Path: "/f"}

	if err := p.Upload(context.Background(), ref, open, 5, "negotiate"); err != nil {
		t.Fatal(err)
	}
	if body != "hello" || !strings.Contains(query, "xs_type=adler32") {
		t.Errorf("body %q, query %q: want hello with the adler32 checksum", body, query)
	}

	err := p.UploadIfUnchanged(context.Background(), ref, open, 5, "old")
	if se, ok := err.(*backend.StatusError); !ok || se.Code != rpc.Code_CODE_FAILED_PRECONDITION {
		t.Errorf("upload with a stale etag: %v, want CODE_FAILED_PRECONDITION", err)
	}
}
