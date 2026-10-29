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

	u, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/big.bin", 5<<20, "")
	if err != nil {
		t.Fatal(err)
	}
	if u != f.ts.URL+"/data/upload-token-123" {
		t.Errorf("upload URL = %q", u)
	}

	req := f.lastRequest(http.MethodPost)
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

	u, err := f.client().CreateUpload(context.Background(), "/eos/user/e/einstein/big.bin", 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if u != f.ts.URL+"/data/upload-token-456" {
		t.Errorf("relative Location was not resolved against the base: %q", u)
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

func TestEncodeTusMetadata(t *testing.T) {
	got := encodeTusMetadata(map[string]string{"filename": "a.txt", "dir": "/eos/user"})
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
