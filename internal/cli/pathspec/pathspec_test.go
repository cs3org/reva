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

package pathspec

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		name      string
		arg       string
		wantSpace string
		wantPath  string
		wantSlash bool
	}{
		{"absolute eos path", "/eos/user/g/gdelmont/Documents", "", "/eos/user/g/gdelmont/Documents", false},
		{"namespace root", "/", "", "/", false},
		{"cb prefix stripped", "cb:/eos/user/g/gdelmont", "", "/eos/user/g/gdelmont", false},
		{"trailing slash recorded", "/eos/project/c/cernbox/", "", "/eos/project/c/cernbox", true},
		{"dot segments collapsed", "/eos/user/g/./gdelmont/../gdelmont/Docs", "", "/eos/user/g/gdelmont/Docs", false},
		{"home alias", "home:Documents", "home", "Documents", false},
		{"home alias with leading slash", "home:/Documents", "home", "Documents", false},
		{"home alias bare", "home:", "home", "", false},
		{"project alias", "project/cernbox:data/raw", "project/cernbox", "data/raw", false},
		{"relative resolves against home", "Documents/notes.txt", "home", "Documents/notes.txt", false},
		{"cb prefix with alias", "cb:home:Documents", "home", "Documents", false},
		{"space id alias", "1284d238-aa92:sub/dir", "1284d238-aa92", "sub/dir", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRemote(tt.arg)
			if err != nil {
				t.Fatalf("ParseRemote(%q) returned error: %v", tt.arg, err)
			}
			if !got.IsRemote() {
				t.Errorf("ParseRemote(%q).Kind = %v, want remote", tt.arg, got.Kind)
			}
			if got.Space != tt.wantSpace {
				t.Errorf("ParseRemote(%q).Space = %q, want %q", tt.arg, got.Space, tt.wantSpace)
			}
			if got.Path != tt.wantPath {
				t.Errorf("ParseRemote(%q).Path = %q, want %q", tt.arg, got.Path, tt.wantPath)
			}
			if got.TrailingSlash != tt.wantSlash {
				t.Errorf("ParseRemote(%q).TrailingSlash = %v, want %v", tt.arg, got.TrailingSlash, tt.wantSlash)
			}
			if got.Raw != tt.arg {
				t.Errorf("ParseRemote(%q).Raw = %q, want the original argument", tt.arg, got.Raw)
			}
		})
	}
}

func TestParseRemoteErrors(t *testing.T) {
	tests := []struct {
		name    string
		arg     string
		wantSub string
	}{
		{"empty", "", "empty path"},
		{"cb prefix with nothing after it", "cb:", "no path after"},
		{"file prefix rejected", "file:./local.txt", "only works on remote paths"},
		{"escaping absolute path", "/eos/../../etc/passwd", "escapes the namespace root"},
		{"escaping alias path", "home:../../etc/passwd", "escapes the space root"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRemote(tt.arg)
			if err == nil {
				t.Fatalf("ParseRemote(%q) succeeded, want an error", tt.arg)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("ParseRemote(%q) error = %q, want it to mention %q", tt.arg, err, tt.wantSub)
			}
		})
	}
}

func TestParseTransferDisambiguation(t *testing.T) {
	// On Windows, Clean gives backslashes, and it keeps a leading ".\" on a name
	// with a colon, so that it is not read as a drive.
	local := filepath.FromSlash
	weird := "weird:name.txt"
	if runtime.GOOS == "windows" {
		weird = `.\weird:name.txt`
	}
	tests := []struct {
		name      string
		arg       string
		wantKind  Kind
		wantPath  string
		wantSpace string
	}{
		{"bare eos path is LOCAL", "/eos/user/g/gdelmont/a.txt", Local, local("/eos/user/g/gdelmont/a.txt"), ""},
		{"cb-marked eos path is remote", "cb:/eos/user/g/gdelmont/a.txt", Remote, "/eos/user/g/gdelmont/a.txt", ""},
		{"relative local path", "./report.pdf", Local, "report.pdf", ""},
		{"plain local path", "report.pdf", Local, "report.pdf", ""},
		{"local absolute path", "/tmp/report.pdf", Local, local("/tmp/report.pdf"), ""},
		{"space alias is remote without cb", "home:Documents", Remote, "Documents", "home"},
		{"project alias is remote", "project/cernbox:data", Remote, "data", "project/cernbox"},
		{"file prefix forces local", "file:home:weird", Local, "home:weird", ""},
		{"windows drive letter stays local", `C:\Users\gdelmont\a.txt`, Local, `C:\Users\gdelmont\a.txt`, ""},
		{"colon inside a relative file name stays local", "./weird:name.txt", Local, weird, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTransfer(tt.arg)
			if err != nil {
				t.Fatalf("ParseTransfer(%q) returned error: %v", tt.arg, err)
			}
			if got.Kind != tt.wantKind {
				t.Errorf("ParseTransfer(%q).Kind = %v, want %v", tt.arg, got.Kind, tt.wantKind)
			}
			if got.Path != tt.wantPath {
				t.Errorf("ParseTransfer(%q).Path = %q, want %q", tt.arg, got.Path, tt.wantPath)
			}
			if got.Space != tt.wantSpace {
				t.Errorf("ParseTransfer(%q).Space = %q, want %q", tt.arg, got.Space, tt.wantSpace)
			}
		})
	}
}

// "data/archive:2024" looks like a project space alias. The file: prefix makes
// it local.
func TestParseTransferAliasAmbiguity(t *testing.T) {
	got, err := ParseTransfer("data/archive:2024")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsRemote() || got.Space != "data/archive" {
		t.Errorf("got %+v, want it read as the space alias data/archive", got)
	}

	escaped, err := ParseTransfer("file:data/archive:2024")
	if err != nil {
		t.Fatal(err)
	}
	if !escaped.IsLocal() || escaped.Path != filepath.FromSlash("data/archive:2024") {
		t.Errorf("file: prefix did not force a local path, got %+v", escaped)
	}
}

func TestParseTransferTrailingSlash(t *testing.T) {
	withSlash, err := ParseTransfer("cb:/eos/user/g/gdelmont/Documents/")
	if err != nil {
		t.Fatal(err)
	}
	if !withSlash.TrailingSlash {
		t.Error("trailing slash was not recorded, cp-style destination semantics depend on it")
	}

	withoutSlash, err := ParseTransfer("cb:/eos/user/g/gdelmont/Documents")
	if err != nil {
		t.Fatal(err)
	}
	if withoutSlash.TrailingSlash {
		t.Error("TrailingSlash set on a path that has none")
	}
	if withSlash.Path != withoutSlash.Path {
		t.Errorf("trailing slash changed the path: %q vs %q", withSlash.Path, withoutSlash.Path)
	}
}

func TestParseTransferPair(t *testing.T) {
	t.Run("local to remote", func(t *testing.T) {
		src, dst, err := ParseTransferPair("./a.txt", "cb:/eos/user/g/gdelmont/")
		if err != nil {
			t.Fatal(err)
		}
		if !src.IsLocal() || !dst.IsRemote() {
			t.Errorf("got src=%v dst=%v, want local → remote", src.Kind, dst.Kind)
		}
	})

	t.Run("remote to local", func(t *testing.T) {
		src, dst, err := ParseTransferPair("cb:/eos/user/g/gdelmont/a.txt", "./")
		if err != nil {
			t.Fatal(err)
		}
		if !src.IsRemote() || !dst.IsLocal() {
			t.Errorf("got src=%v dst=%v, want remote → local", src.Kind, dst.Kind)
		}
	})

	t.Run("two bare eos paths are rejected with advice", func(t *testing.T) {
		_, _, err := ParseTransferPair("/eos/user/g/gdelmont/a.txt", "/eos/user/g/gdelmont/b.txt")
		if err == nil {
			t.Fatal("expected an error for two unmarked paths")
		}
		if !strings.Contains(err.Error(), "cb:") {
			t.Errorf("error should suggest the cb: prefix, got %q", err)
		}
	})

	t.Run("two remote paths are allowed", func(t *testing.T) {
		src, dst, err := ParseTransferPair("cb:/eos/a", "home:b")
		if err != nil {
			t.Fatal(err)
		}
		if !src.IsRemote() || !dst.IsRemote() {
			t.Error("both sides should be remote")
		}
	})
}
