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

package client

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

func TestCreateUpload(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", f.ts.URL+"/data/upload-token-123")
		w.Header().Set(hdrTusResumable, tusVersion)
		w.WriteHeader(http.StatusCreated)
	})

	up, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/big.bin", 5<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	if up.URL != f.ts.URL+"/data/upload-token-123" {
		t.Errorf("upload URL = %q", up.URL)
	}
	if up.Size != 5<<20 {
		t.Errorf("Size = %d", up.Size)
	}

	req := f.lastRequest(http.MethodPost)
	// The TUS creation POST goes to the parent directory, not the file.
	if !strings.HasSuffix(req.Path, "/eos/user/e/einstein") {
		t.Errorf("POST path = %q, want the parent directory", req.Path)
	}
	if got := req.Header.Get(hdrTusResumable); got != tusVersion {
		t.Errorf("Tus-Resumable = %q", got)
	}
	if got := req.Header.Get(hdrUploadLength); got != "5242880" {
		t.Errorf("Upload-Length = %q", got)
	}

	meta := parseTusMetadata(t, req.Header.Get(hdrUploadMetadata))
	if meta["filename"] != "big.bin" {
		t.Errorf("filename metadata = %q", meta["filename"])
	}
	if meta["dir"] != "/eos/user/e/einstein" {
		t.Errorf("dir metadata = %q", meta["dir"])
	}
}

func TestCreateUploadRelativeLocation(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/data/upload-token-456")
		w.WriteHeader(http.StatusCreated)
	})

	up, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/big.bin", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if up.URL != f.ts.URL+"/data/upload-token-456" {
		t.Errorf("relative Location was not resolved against the base: %q", up.URL)
	}
}

func TestCreateUploadWithoutLocationIsAnError(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPost, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	_, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/big.bin", 10, "")
	if err == nil || !strings.Contains(err.Error(), "no Location header") {
		t.Errorf("got %v, want a clear error about the missing Location", err)
	}
}

func TestUploadOffset(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodHead, "/data/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hdrUploadOffset, "1048576")
		w.WriteHeader(http.StatusOK)
	})

	up := &Upload{URL: f.ts.URL + "/data/token", Path: "/eos/user/e/einstein/big.bin", Size: 5 << 20}
	offset, err := f.client().UploadOffset(context.Background(), up)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 1048576 {
		t.Errorf("offset = %d, want 1048576", offset)
	}
}

func TestUploadOffsetWithoutHeaderIsAnError(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodHead, "/data/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	up := &Upload{URL: f.ts.URL + "/data/token"}
	if _, err := f.client().UploadOffset(context.Background(), up); err == nil {
		t.Error("expected an error when the server reports no offset")
	}
}

func TestUploadChunk(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPatch, "/data/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(hdrUploadOffset, "2048")
		w.WriteHeader(http.StatusNoContent)
	})

	up := &Upload{URL: f.ts.URL + "/data/token", Path: "/eos/user/e/einstein/big.bin"}
	offset, err := f.client().UploadChunk(context.Background(), up, 1024, openerOf(strings.Repeat("x", 1024)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 2048 {
		t.Errorf("offset = %d, want 2048", offset)
	}

	req := f.lastRequest(http.MethodPatch)
	if got := req.Header.Get(hdrUploadOffset); got != "1024" {
		t.Errorf("sent Upload-Offset = %q, want 1024", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/offset+octet-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if len(req.Body) != 1024 {
		t.Errorf("body length = %d, want 1024", len(req.Body))
	}
}

// TestUploadChunkTrustsServerOffset is a data-integrity guard. If the server
// accepted fewer bytes than we sent, continuing from our own arithmetic would
// leave a hole in the file and the upload would complete "successfully" with
// corrupt content.
func TestUploadChunkTrustsServerOffset(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPatch, "/data/", func(w http.ResponseWriter, r *http.Request) {
		// The server accepted only 500 of the 1024 bytes offered.
		w.Header().Set(hdrUploadOffset, "1524")
		w.WriteHeader(http.StatusNoContent)
	})

	up := &Upload{URL: f.ts.URL + "/data/token"}
	offset, err := f.client().UploadChunk(context.Background(), up, 1024, openerOf(strings.Repeat("x", 1024)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 1524 {
		t.Errorf("offset = %d, want the server's 1524 rather than our computed 2048", offset)
	}
}

func TestUploadChunkFallsBackToComputedOffset(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPatch, "/data/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	up := &Upload{URL: f.ts.URL + "/data/token"}
	offset, err := f.client().UploadChunk(context.Background(), up, 1024, openerOf("xxxx"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 1028 {
		t.Errorf("offset = %d, want 1028 when the server omits the header", offset)
	}
}

func TestEncodeTusMetadata(t *testing.T) {
	got := encodeTusMetadata(map[string]string{"filename": "a.txt", "dir": "/eos/user"})
	// Deterministic ordering: keys sorted, so "dir" comes first.
	want := "dir " + base64.StdEncoding.EncodeToString([]byte("/eos/user")) +
		",filename " + base64.StdEncoding.EncodeToString([]byte("a.txt"))
	if got != want {
		t.Errorf("encodeTusMetadata = %q, want %q", got, want)
	}
}

func TestEncodeTusMetadataEmptyValue(t *testing.T) {
	if got := encodeTusMetadata(map[string]string{"flag": ""}); got != "flag" {
		t.Errorf("an empty value should produce a bare key, got %q", got)
	}
}

func TestCreateUploadRejectsDirectoryPath(t *testing.T) {
	f := newFakeServer(t)
	if _, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/", 10, ""); err == nil {
		t.Error("a path with no file name should be rejected")
	}
}

func parseTusMetadata(t *testing.T, header string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for pair := range strings.SplitSeq(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), " ")
		if !ok {
			out[key] = ""
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatalf("metadata value for %q is not base64: %v", key, err)
		}
		out[key] = string(decoded)
	}
	return out
}

func TestUploadChunkSendsContentLength(t *testing.T) {
	f := newFakeServer(t)
	var gotLength int64 = -1
	var chunked bool
	f.on(http.MethodPatch, "/data/", func(w http.ResponseWriter, r *http.Request) {
		gotLength = r.ContentLength
		chunked = len(r.TransferEncoding) > 0 && r.TransferEncoding[0] == "chunked"
		w.Header().Set(hdrUploadOffset, "1024")
		w.WriteHeader(http.StatusNoContent)
	})

	up := &Upload{URL: f.ts.URL + "/data/token"}
	body := strings.Repeat("x", 1024)
	if _, err := f.client().UploadChunk(context.Background(), up, 0, openerOf(body), 1024); err != nil {
		t.Fatal(err)
	}
	if chunked {
		t.Error("the chunk was sent with chunked transfer encoding")
	}
	if gotLength != 1024 {
		t.Errorf("server saw ContentLength %d, want 1024", gotLength)
	}
}
