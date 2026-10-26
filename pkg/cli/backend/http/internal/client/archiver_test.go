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
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func TestArchive(t *testing.T) {
	f := newFakeServer(t)
	var gotQuery url.Values
	f.on(http.MethodGet, "/archiver", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Write([]byte("tar-bytes"))
	})

	rc, err := f.client().Archive(context.Background(),
		[]string{"/eos/user/e/einstein/Documents", "/eos/user/e/einstein/notes.txt"}, ArchiveTar)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	body, _ := io.ReadAll(rc)
	if string(body) != "tar-bytes" {
		t.Errorf("archive body = %q", body)
	}

	paths := gotQuery["path"]
	if len(paths) != 2 {
		t.Fatalf("query paths = %v, want both paths as repeated parameters", paths)
	}
	if paths[0] != "/eos/user/e/einstein/Documents" {
		t.Errorf("first path = %q", paths[0])
	}
	if gotQuery.Get("arch_type") != "tar" {
		t.Errorf("arch_type = %q", gotQuery.Get("arch_type"))
	}
}

func TestArchiveRejectsUnsupportedFormat(t *testing.T) {
	f := newFakeServer(t)
	_, err := f.client().Archive(context.Background(), []string{"/eos/x"}, "7z")
	if err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("kind = %v, want usage", cberr.KindOf(err))
	}
	if !strings.Contains(err.Error(), "tar") {
		t.Errorf("the error should list what the server does support, got %q", err)
	}
}

// TestArchiveWhenDisabled: the caller needs to know to fall back to a
// file-by-file walk rather than failing the whole download.
func TestArchiveWhenDisabled(t *testing.T) {
	f := newFakeServer(t)
	f.capabilitiesJSON = `{"ocs":{"data":{"capabilities":{"core":{}}}}}`

	_, err := f.client().Archive(context.Background(), []string{"/eos/x"}, ArchiveTar)
	if err == nil || !strings.Contains(err.Error(), "no archiver service") {
		t.Errorf("got %v, want a clear 'no archiver' error", err)
	}
}

func TestArchiveRejectsEmptyPathList(t *testing.T) {
	f := newFakeServer(t)
	if _, err := f.client().Archive(context.Background(), nil, ArchiveTar); err == nil {
		t.Error("expected an error for an empty path list")
	}
}

func TestArchiveAppendsToExistingQuery(t *testing.T) {
	f := newFakeServer(t)
	f.capabilitiesJSON = strings.Replace(defaultCapabilities,
		`"archiver_url": "/archiver"`, `"archiver_url": "/archiver?v=2"`, 1)

	var gotQuery url.Values
	f.on(http.MethodGet, "/archiver", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Write([]byte("ok"))
	})

	rc, err := f.client().Archive(context.Background(), []string{"/eos/x"}, ArchiveZip)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()

	if gotQuery.Get("v") != "2" {
		t.Errorf("the archiver URL's own query was dropped: %v", gotQuery)
	}
	if gotQuery.Get("arch_type") != "zip" {
		t.Errorf("arch_type = %q", gotQuery.Get("arch_type"))
	}
}
