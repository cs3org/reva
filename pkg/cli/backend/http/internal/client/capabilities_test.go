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
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestCapabilities(t *testing.T) {
	f := newFakeServer(t)
	caps, err := f.client().Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if !caps.TusSupported {
		t.Error("TusSupported = false, want true")
	}
	if caps.TusMaxChunkSize != 1048576 {
		t.Errorf("TusMaxChunkSize = %d", caps.TusMaxChunkSize)
	}
	if caps.PreferredChecksum != "md5" {
		t.Errorf("PreferredChecksum = %q", caps.PreferredChecksum)
	}
	if !caps.ArchiverEnabled {
		t.Error("ArchiverEnabled = false, want true")
	}
	if caps.ArchiverURL != "/archiver" {
		t.Errorf("ArchiverURL = %q", caps.ArchiverURL)
	}
	if caps.ArchiverMaxNumFiles != 10000 {
		t.Errorf("ArchiverMaxNumFiles = %d, want the string field parsed to 10000", caps.ArchiverMaxNumFiles)
	}
	if caps.ArchiverMaxSize != 1073741824 {
		t.Errorf("ArchiverMaxSize = %d", caps.ArchiverMaxSize)
	}
	if !caps.Versioning || !caps.Undelete || !caps.Sharing || !caps.PublicLinks || !caps.SpacesEnabled {
		t.Errorf("feature flags are wrong: %+v", caps)
	}
	if caps.ServerVersion != "10.0.11" {
		t.Errorf("ServerVersion = %q", caps.ServerVersion)
	}
}

func TestCapabilitiesFetchedOnce(t *testing.T) {
	f := newFakeServer(t)
	var calls atomic.Int32
	f.on(http.MethodGet, ocsCapabilities, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, defaultCapabilities)
	})

	c := f.client()
	for range 3 {
		if _, err := c.Capabilities(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("capabilities fetched %d times, want 1", got)
	}
}

// TestOcsBoolShapes: OCS serialises booleans as true, "1", or 1 depending on
// the field, so a plain bool would fail to decode half the document.
func TestOcsBoolShapes(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{`true`, true},
		{`"1"`, true},
		{`1`, true},
		{`"true"`, true},
		{`false`, false},
		{`"0"`, false},
		{`0`, false},
		{`null`, false},
		{`""`, false},
	}
	for _, tt := range tests {
		var b ocsBool
		if err := json.Unmarshal([]byte(tt.in), &b); err != nil {
			t.Errorf("ocsBool(%s): %v", tt.in, err)
			continue
		}
		if bool(b) != tt.want {
			t.Errorf("ocsBool(%s) = %v, want %v", tt.in, bool(b), tt.want)
		}
	}
}

func TestCapabilitiesWithoutOptionalSections(t *testing.T) {
	f := newFakeServer(t)
	f.capabilitiesJSON = `{"ocs":{"meta":{"status":"ok"},"data":{"capabilities":{"core":{}}}}}`

	caps, err := f.client().Capabilities(context.Background())
	if err != nil {
		t.Fatalf("a minimal capabilities document should not be an error: %v", err)
	}
	if caps.TusSupported {
		t.Error("TusSupported should be false when the server does not advertise it")
	}
	if caps.ArchiverEnabled {
		t.Error("ArchiverEnabled should be false when there are no archivers")
	}
}

func TestCapabilitiesIgnoresDisabledArchiver(t *testing.T) {
	f := newFakeServer(t)
	f.capabilitiesJSON = `{"ocs":{"data":{"capabilities":{"files":{"archivers":[
	  {"enabled": false, "formats": ["tar"], "archiver_url": "/archiver"}
	]}}}}}`

	caps, err := f.client().Capabilities(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if caps.ArchiverEnabled {
		t.Error("a disabled archiver should not be used")
	}
}

func TestChunkSize(t *testing.T) {
	tests := []struct {
		name      string
		caps      *Capabilities
		preferred int64
		want      int64
	}{
		{"clamped to server maximum", &Capabilities{TusMaxChunkSize: 1 << 20}, 1 << 24, 1 << 20},
		{"preference respected", &Capabilities{TusMaxChunkSize: 1 << 24}, 1 << 20, 1 << 20},
		{"no server limit", &Capabilities{}, 1 << 20, 1 << 20},
		{"no preference", &Capabilities{TusMaxChunkSize: 1 << 20}, 0, 1 << 20},
		{"nil capabilities", nil, 1 << 20, 1 << 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.ChunkSize(tt.preferred); got != tt.want {
				t.Errorf("ChunkSize(%d) = %d, want %d", tt.preferred, got, tt.want)
			}
		})
	}
}

func TestBestChecksum(t *testing.T) {
	tests := []struct {
		name string
		caps *Capabilities
		want string
	}{
		{"server preference wins", &Capabilities{PreferredChecksum: "adler32", ChecksumTypes: []string{"sha1", "adler32"}}, "adler32"},
		{"falls back to strongest supported", &Capabilities{ChecksumTypes: []string{"md5", "adler32"}}, "md5"},
		{"sha1 preferred over md5", &Capabilities{ChecksumTypes: []string{"adler32", "md5", "sha1"}}, "sha1"},
		{"none supported", &Capabilities{}, ""},
		{"nil", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.caps.BestChecksum(); got != tt.want {
				t.Errorf("BestChecksum() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSupportsArchiveFormat(t *testing.T) {
	caps := &Capabilities{ArchiverEnabled: true, ArchiverFormats: []string{"tar", "zip"}}
	if !caps.SupportsArchiveFormat("tar") || !caps.SupportsArchiveFormat("zip") {
		t.Error("tar and zip should both be supported")
	}
	if caps.SupportsArchiveFormat("7z") {
		t.Error("7z should not be supported")
	}
	if (&Capabilities{ArchiverFormats: []string{"tar"}}).SupportsArchiveFormat("tar") {
		t.Error("a disabled archiver supports nothing")
	}
}

func TestArchiveFits(t *testing.T) {
	caps := &Capabilities{ArchiverEnabled: true, ArchiverMaxNumFiles: 100, ArchiverMaxSize: 1000}

	if !caps.ArchiveFits(50, 500) {
		t.Error("a small tree should fit")
	}
	if caps.ArchiveFits(150, 500) {
		t.Error("too many files should not fit")
	}
	if caps.ArchiveFits(50, 5000) {
		t.Error("too many bytes should not fit")
	}

	unlimited := &Capabilities{ArchiverEnabled: true}
	if !unlimited.ArchiveFits(1e9, 1e15) {
		t.Error("a server advertising no limits should always fit")
	}
}
