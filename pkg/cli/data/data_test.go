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
	"strconv"
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

func (l locator) ResumableTarget(context.Context, *provider.Reference, int64, string) (*backend.Target, error) {
	return &backend.Target{URL: l.url + "/tus/1", Header: http.Header{"X-Reva-Transfer": {"tx"}}}, nil
}

func (l locator) VersionTarget(_ context.Context, _ *provider.Reference, key string) (*backend.Target, error) {
	return &backend.Target{URL: l.url + "/v/" + key, Header: http.Header{"X-Reva-Transfer": {"tx"}}}, nil
}

func (l locator) ArchiveTarget(context.Context, []*provider.Reference, string) (*backend.Target, error) {
	return nil, backend.NotAvailable("test", "archive")
}

func (l locator) Capabilities(context.Context) (*backend.Capabilities, error) {
	return &backend.Capabilities{TUS: true, MaxChunkSize: 4}, nil
}

func (l locator) AuthHeader(context.Context) (http.Header, error) {
	return http.Header{"X-Reva-Transfer": {"tx"}}, nil
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

// tusServer keeps one upload, and refuses a part at the offset failAt once.
type tusServer struct {
	data   []byte
	failAt int64
}

func (ts *tusServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Reva-Transfer") != "tx" || r.Header.Get("Tus-Resumable") == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodHead:
		w.Header().Set("Upload-Offset", strconv.Itoa(len(ts.data)))
	case http.MethodPatch:
		off, _ := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
		if off != int64(len(ts.data)) {
			http.Error(w, "wrong offset", http.StatusConflict)
			return
		}
		if off == ts.failAt {
			ts.failAt = -1
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		b, _ := io.ReadAll(r.Body)
		ts.data = append(ts.data, b...)
		w.Header().Set("Upload-Offset", strconv.Itoa(len(ts.data)))
		w.WriteHeader(http.StatusNoContent)
	}
}

func TestUploadResumable(t *testing.T) {
	ts := &tusServer{failAt: 8}
	srv := httptest.NewServer(ts)
	defer srv.Close()
	p := New(srv.Client(), locator{url: srv.URL})
	open := func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("0123456789")), nil }
	ref := &provider.Reference{Path: "/f"}
	var last backend.Upload
	err := p.UploadResumable(context.Background(), ref, open, 10, "", func(u backend.Upload) { last = u })
	if err == nil || string(ts.data) != "01234567" || last.Offset != 8 {
		t.Fatalf("first run: err %v, server has %q, state %+v: want a failure after 8 bytes", err, ts.data, last)
	}
	if err := p.ResumeUpload(context.Background(), last, open, func(u backend.Upload) { last = u }); err != nil {
		t.Fatal(err)
	}
	if string(ts.data) != "0123456789" || last.Offset != 10 {
		t.Errorf("after resume the server has %q, state %+v: want the whole file", ts.data, last)
	}
}

func TestRenewAfterUnauthorized(t *testing.T) {
	token := "old"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "new" {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		io.WriteString(w, "content of "+r.URL.Path)
	}))
	defer srv.Close()
	renewed := 0
	loc := renewLocator{url: srv.URL, token: &token}
	p := New(srv.Client(), loc)
	p.Renew = func(context.Context) error { renewed++; token = "new"; return nil }
	body, _, err := p.DownloadVersion(context.Background(), &provider.Reference{Path: "/f"}, "v1")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	body.Close()
	if string(got) != "content of /v/v1" || renewed != 1 {
		t.Errorf("got %q after %d renewals, want the version after one", got, renewed)
	}
}

type renewLocator struct {
	locator
	token *string
}

func (l renewLocator) VersionTarget(ctx context.Context, ref *provider.Reference, key string) (*backend.Target, error) {
	t, _ := l.locator.VersionTarget(ctx, ref, key)
	t.URL += "?t=" + *l.token
	return t, nil
}

func TestUploadChecksumHeader(t *testing.T) {
	var header, query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header, query = r.Header.Get("OC-Checksum"), r.URL.RawQuery
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	p := New(srv.Client(), davLocator{locator{url: srv.URL, checksums: []*provider.ResourceChecksumPriority{
		{Type: provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32},
	}}})
	open := func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("hello")), nil }
	if err := p.Upload(context.Background(), &provider.Reference{Path: "/f"}, open, 5, "negotiate"); err != nil {
		t.Fatal(err)
	}
	if header != "ADLER32:062c0215" || query != "" {
		t.Errorf("OC-Checksum %q, query %q: want the adler32 in the header only", header, query)
	}
}

type davLocator struct{ locator }

func (l davLocator) UploadTarget(ctx context.Context, ref *provider.Reference, size int64, ifMatch string) (*backend.Target, error) {
	t, err := l.locator.UploadTarget(ctx, ref, size, ifMatch)
	t.ChecksumHeader = true
	return t, err
}
