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
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

const testResourceID = "localhome$ABC!fileid-1"

func versionsMultistatus(resourceID string, keys ...versionFixture) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">`)

	sb.WriteString(`<d:response><d:href>` + davMetaPrefix + "/" + resourceID + `/v</d:href>`)
	sb.WriteString(`<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>`)
	sb.WriteString(`<d:resourcetype><d:collection/></d:resourcetype>`)
	sb.WriteString(`</d:prop></d:propstat></d:response>`)

	for _, k := range keys {
		sb.WriteString(`<d:response><d:href>` + davMetaPrefix + "/" + resourceID + `/v/` + k.Key + `</d:href>`)
		sb.WriteString(`<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>`)
		sb.WriteString(`<d:resourcetype></d:resourcetype>`)
		fmt.Fprintf(&sb, "<d:getcontentlength>%d</d:getcontentlength>", k.Size)
		if k.Modified != "" {
			sb.WriteString(`<d:getlastmodified>` + k.Modified + `</d:getlastmodified>`)
		}
		sb.WriteString(`<d:getetag>&quot;` + k.ETag + `&quot;</d:getetag>`)
		sb.WriteString(`</d:prop></d:propstat></d:response>`)
	}
	sb.WriteString(`</d:multistatus>`)
	return sb.String()
}

type versionFixture struct {
	Key      string
	Size     int64
	Modified string
	ETag     string
}

func TestListVersions(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davMetaPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, versionsMultistatus(testResourceID,
			versionFixture{Key: "1767139200", Size: 100, Modified: "Wed, 31 Dec 2025 00:00:00 GMT", ETag: "v1"},
			versionFixture{Key: "1767225600", Size: 200, Modified: "Thu, 01 Jan 2026 00:00:00 GMT", ETag: "v2"},
		))
	})

	versions, err := f.client().ListVersions(context.Background(), testResourceID)
	if err != nil {
		t.Fatal(err)
	}

	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2: %+v", len(versions), versions)
	}
	if versions[0].Key != "1767225600" {
		t.Errorf("first version = %q, want the newest", versions[0].Key)
	}
	if versions[0].Size != 200 || versions[0].ETag != "v2" {
		t.Errorf("version = %+v", versions[0])
	}
	if versions[0].Modified.Before(versions[1].Modified) {
		t.Error("versions are not sorted newest first")
	}
}

func TestListVersionsFallsBackToTheKeyForTime(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davMetaPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, versionsMultistatus(testResourceID,
			versionFixture{Key: "1767225600", Size: 200, ETag: "v2"},
		))
	})

	versions, err := f.client().ListVersions(context.Background(), testResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("got %d versions", len(versions))
	}
	if !versions[0].Modified.Equal(time.Unix(1767225600, 0)) {
		t.Errorf("Modified = %v, want it derived from the key", versions[0].Modified)
	}
}

func TestListVersionsEmpty(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davMetaPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, versionsMultistatus(testResourceID))
	})

	versions, err := f.client().ListVersions(context.Background(), testResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Errorf("got %d versions for a file with no history", len(versions))
	}
}

func TestVersionKeyFromHref(t *testing.T) {
	tests := []struct {
		href string
		want string
	}{
		{davMetaPrefix + "/id/v/1767225600", "1767225600"},
		{davMetaPrefix + "/id/v/1767225600/", "1767225600"},
		{davMetaPrefix + "/id/v", ""},
		{"/somewhere/else", ""},
	}
	for _, tt := range tests {
		if got := versionKeyFromHref(tt.href); got != tt.want {
			t.Errorf("versionKeyFromHref(%q) = %q, want %q", tt.href, got, tt.want)
		}
	}
}

func TestRestoreVersionUsesCopy(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodCopy, davMetaPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := f.client().RestoreVersion(context.Background(), testResourceID, "1767225600"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.lastRequest(MethodCopy).Path, "/v/1767225600") {
		t.Errorf("restore path = %q", f.lastRequest(MethodCopy).Path)
	}
}

func TestRestoreVersionRequiresAKey(t *testing.T) {
	f := newFakeServer(t)
	err := f.client().RestoreVersion(context.Background(), testResourceID, "")
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error", err)
	}
}

func TestEpochToTime(t *testing.T) {
	const seconds int64 = 1767225600 // 2026-01-01
	const millis = seconds * 1000

	if got := epochToTime(seconds); got.Year() != 2026 {
		t.Errorf("epochToTime(%d) = %v, want a 2026 date", seconds, got)
	}
	if got := epochToTime(millis); got.Year() != 2026 {
		t.Errorf("epochToTime(%d) = %v, want the same 2026 date from milliseconds", millis, got)
	}
	if got := epochToTime(0); !got.IsZero() {
		t.Errorf("epochToTime(0) = %v, want the zero time", got)
	}
	if got := epochToTime(-1); !got.IsZero() {
		t.Errorf("epochToTime(-1) = %v, want the zero time", got)
	}
}

func TestListVersionsWithMillisecondKeys(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davMetaPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, versionsMultistatus(testResourceID,
			versionFixture{Key: "1767225600000", Size: 10, ETag: "v1"},
		))
	})

	versions, err := f.client().ListVersions(context.Background(), testResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("got %d versions", len(versions))
	}
	if year := versions[0].Modified.Year(); year != 2026 {
		t.Errorf("version year = %d, want 2026: the millisecond key was read as seconds", year)
	}
}
