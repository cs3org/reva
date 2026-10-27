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

package transfer

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tarball builds a tar stream from a list of entries.
func tarball(t *testing.T, entries []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, hdr := range entries {
		h := hdr
		if body, ok := bodies[h.Name]; ok {
			h.Size = int64(len(body))
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if body, ok := bodies[h.Name]; ok {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractTar(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	data := tarball(t,
		[]tar.Header{
			{Name: "data", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "data/a.txt", Typeflag: tar.TypeReg, Mode: 0o644},
			{Name: "data/sub", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "data/sub/b.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		},
		map[string]string{"data/a.txt": "alpha", "data/sub/b.txt": "beta"},
	)

	dir := t.TempDir()
	stats, err := e.extractTar(context.Background(), bytes.NewReader(data), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Dirs != 2 {
		t.Errorf("stats = %+v, want 2 files and 2 directories", stats)
	}
	if stats.Bytes != 9 {
		t.Errorf("Bytes = %d, want 9", stats.Bytes)
	}
	assertFile(t, filepath.Join(dir, "data", "a.txt"), "alpha")
	assertFile(t, filepath.Join(dir, "data", "sub", "b.txt"), "beta")
}

// TestExtractTarRejectsPathTraversal is the archive equivalent of zip-slip: an
// entry named ../../etc/x must not be written outside the destination, no
// matter what the server sends.
func TestExtractTarRejectsPathTraversal(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	for _, name := range []string{
		"../escaped.txt",
		"../../etc/passwd",
		"data/../../escaped.txt",
		"/absolute.txt",
	} {
		t.Run(name, func(t *testing.T) {
			data := tarball(t,
				[]tar.Header{{Name: name, Typeflag: tar.TypeReg, Mode: 0o644}},
				map[string]string{name: "malicious"},
			)

			parent := t.TempDir()
			dest := filepath.Join(parent, "dest")

			_, err := e.extractTar(context.Background(), bytes.NewReader(data), dest, "")
			if err == nil {
				// An absolute path is neutralised by Join rather than rejected,
				// so accept either outcome as long as nothing escaped.
				if _, statErr := os.Stat(filepath.Join(parent, "escaped.txt")); statErr == nil {
					t.Fatal("an entry was written outside the destination")
				}
				return
			}
			if !strings.Contains(err.Error(), "outside the destination") {
				t.Errorf("got %v, want a clear traversal rejection", err)
			}
			if _, statErr := os.Stat(filepath.Join(parent, "escaped.txt")); statErr == nil {
				t.Error("an entry was written outside the destination")
			}
		})
	}
}

// TestExtractTarSkipsLinks: a link entry could point anywhere, and the server does
// not store symlinks anyway.
func TestExtractTarSkipsLinks(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	data := tarball(t,
		[]tar.Header{
			{Name: "ok.txt", Typeflag: tar.TypeReg, Mode: 0o644},
			{Name: "evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777},
		},
		map[string]string{"ok.txt": "fine"},
	)

	dir := t.TempDir()
	stats, err := e.extractTar(context.Background(), bytes.NewReader(data), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 {
		t.Errorf("Files = %d, want 1", stats.Files)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want the symlink skipped", stats.Skipped)
	}
	if _, err := os.Lstat(filepath.Join(dir, "evil")); err == nil {
		t.Error("a symlink entry was created")
	}
}

func TestExtractTarSkipsExistingWithoutOverwrite(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{})

	data := tarball(t,
		[]tar.Header{{Name: "a.txt", Typeflag: tar.TypeReg, Mode: 0o644}},
		map[string]string{"a.txt": "from archive"},
	)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	stats, err := e.extractTar(context.Background(), bytes.NewReader(data), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	assertFile(t, filepath.Join(dir, "a.txt"), "local")
}

func TestExtractTarTruncatedStream(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	// The body must be large enough that the cut lands inside the file
	// content: a tar stream truncated exactly at a block boundary after a
	// complete entry is indistinguishable from a short archive.
	body := strings.Repeat("x", 8<<10)
	data := tarball(t,
		[]tar.Header{{Name: "a.txt", Typeflag: tar.TypeReg, Mode: 0o644}},
		map[string]string{"a.txt": body},
	)

	dir := t.TempDir()
	_, err := e.extractTar(context.Background(), bytes.NewReader(data[:len(data)/2]), dir, "")
	if err == nil {
		t.Fatal("a truncated archive should be reported, not silently accepted")
	}

	// Whatever was written must not be presented as a complete file.
	if got, readErr := os.ReadFile(filepath.Join(dir, "a.txt")); readErr == nil && len(got) == len(body) {
		t.Error("a truncated entry was written out at full length")
	}
}

func TestSafeJoin(t *testing.T) {
	root := filepath.FromSlash("/tmp/dest")

	good := map[string]string{
		"a.txt":        "/tmp/dest/a.txt",
		"sub/b.txt":    "/tmp/dest/sub/b.txt",
		"./sub/c.txt":  "/tmp/dest/sub/c.txt",
		"sub/../d.txt": "/tmp/dest/d.txt",
		"sub//e.txt":   "/tmp/dest/sub/e.txt",
	}
	for name, want := range good {
		want = filepath.FromSlash(want)
		got, err := safeJoin(root, name)
		if err != nil {
			t.Errorf("safeJoin(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("safeJoin(%q) = %q, want %q", name, got, want)
		}
	}

	bad := []string{"../escape", "../../escape", "sub/../../escape", ".."}
	if runtime.GOOS == "windows" {
		// A drive, an alternate data stream, or a device rather than a file.
		bad = append(bad, `C:\escape`, "C:escape", "a:stream", "NUL", "sub/con")
	}
	for _, name := range bad {
		if _, err := safeJoin(root, name); err == nil {
			t.Errorf("safeJoin(%q) should be rejected", name)
		}
	}
}

func TestArchiveDownloadNeedsTar(t *testing.T) {
	box := newFakeBox(t)
	box.archiverEnabled = false
	e := box.engine(Options{Overwrite: true, Archive: true})

	_, err := e.archiveDownload(context.Background(), "/eos/user/e/einstein/data", t.TempDir())
	if err == nil {
		t.Fatal("expected an error with no archiver")
	}
}

// TestExtractTarStripsTheArchiverWrapper: the archiver names its top directory
// after the request, and the destination already stands for it.
func TestExtractTarStripsTheArchiverWrapper(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	data := tarball(t,
		[]tar.Header{
			{Name: "tree/", Typeflag: tar.TypeDir, Mode: 0o755},
			{Name: "tree/a.txt", Typeflag: tar.TypeReg, Mode: 0o644},
			{Name: "tree/sub/b.txt", Typeflag: tar.TypeReg, Mode: 0o644},
		},
		map[string]string{"tree/a.txt": "a", "tree/sub/b.txt": "bb"},
	)

	dir := t.TempDir()
	if _, err := e.extractTar(context.Background(), bytes.NewReader(data), dir, "tree"); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(dir, "a.txt"), "a")
	assertFile(t, filepath.Join(dir, "sub", "b.txt"), "bb")
	if _, err := os.Stat(filepath.Join(dir, "tree")); err == nil {
		t.Error("the wrapper directory was recreated inside the destination")
	}
}

// TestStripArchiveRootOnlyRemovesTheNamedWrapper guards the obvious wrong fix:
// stripping the first component of every entry regardless of what it is.
func TestStripArchiveRootOnlyRemovesTheNamedWrapper(t *testing.T) {
	tests := []struct {
		name, strip, want string
	}{
		{"tree/a.txt", "tree", "a.txt"},
		{"tree/sub/b.txt", "tree", "sub/b.txt"},
		{"tree", "tree", ""},
		{"tree/", "tree", ""},
		// Not the wrapper: a real directory that must survive.
		{"other/a.txt", "tree", "other/a.txt"},
		// No wrapper expected at all, as when a test builds a flat archive.
		{"a.txt", "", "a.txt"},
		{"sub/b.txt", "", "sub/b.txt"},
		// A prefix that merely looks similar is not the wrapper.
		{"treehouse/a.txt", "tree", "treehouse/a.txt"},
	}
	for _, tt := range tests {
		if got := stripArchiveRoot(tt.name, tt.strip); got != tt.want {
			t.Errorf("stripArchiveRoot(%q, %q) = %q, want %q", tt.name, tt.strip, got, tt.want)
		}
	}
}
