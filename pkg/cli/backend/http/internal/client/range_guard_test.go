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
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// framedRange writes a ranged response like EOS does when it fails: the
// chunked framing is inside the body and counts in the length.
func framedRange(w http.ResponseWriter, part string) {
	body := (fmt.Sprintf("%x\r\n", len(part)) + part)[:len(part)]
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusPartialContent)
	fmt.Fprint(w, body)
}

// Without this guard a resumed download ends with a file of the right length
// and wrong bytes.
func TestDownloadRefusesAChunkFramedRange(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		framedRange(w, strings.Repeat("line of log\n", 10))
	})

	_, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/log.txt", 24)
	if err == nil {
		t.Fatal("a body carrying chunked framing must be refused, not returned")
	}
	if !strings.Contains(err.Error(), "chunked framing") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}
	if !strings.Contains(err.Error(), "storage") {
		t.Errorf("the error does not say where the fault is: %v", err)
	}
}

// The bytes that the check peeks at must come back in front of the rest.
func TestDownloadPassesASoundRangeThrough(t *testing.T) {
	want := strings.Repeat("line of log\n", 10)
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, want)
	})

	body, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/log.txt", 24)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDownloadPassesARangeShorterThanTheMarker(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, "h")
	})

	body, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/a.txt", 7)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != "h" {
		t.Errorf("got %q, want %q", got, "h")
	}
}

// A download without a Range is not checked.
func TestDownloadWithoutAnOffsetIsNotGuarded(t *testing.T) {
	// This body starts with its own length in hex. The check would refuse it.
	want := "7\r\nabcdef"
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(want)))
		fmt.Fprint(w, want)
	})

	body, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/a.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	got, _ := io.ReadAll(body)
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
