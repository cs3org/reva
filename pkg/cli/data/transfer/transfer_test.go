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
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func writeLocal(t *testing.T, dir, rel string, body []byte) string {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return full
}

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}
	return b
}

func TestUploadSmallFileUsesPut(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	e := box.engine(Options{Overwrite: true})

	local := writeLocal(t, t.TempDir(), "notes.txt", []byte("hello cernbox"))
	n, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n != 13 {
		t.Errorf("uploaded %d bytes, want 13", n)
	}
	box.requireContent("/eos/user/e/einstein/notes.txt", []byte("hello cernbox"))

	// A small file should not pay for two extra TUS round trips.
	if box.putCount != 1 {
		t.Errorf("PUT count = %d, want 1", box.putCount)
	}
	if box.tusPostCount != 0 {
		t.Errorf("TUS was used for a small file (%d creations)", box.tusPostCount)
	}
}

func TestUploadLargeFileUsesTus(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	box.maxChunk = 64 << 10
	e := box.engine(Options{Overwrite: true, ChunkSize: 64 << 10, PutThreshold: 1 << 10})

	body := payload(5 << 20)
	local := writeLocal(t, t.TempDir(), "big.bin", body)

	n, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)) {
		t.Errorf("uploaded %d bytes, want %d", n, len(body))
	}
	box.requireContent("/eos/user/e/einstein/big.bin", body)

	if box.tusPostCount != 1 {
		t.Errorf("TUS creations = %d, want 1", box.tusPostCount)
	}
	if box.patchCount < 2 {
		t.Errorf("PATCH count = %d, want the file split into several chunks", box.patchCount)
	}
}

func TestUploadRespectsServerChunkLimit(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	box.maxChunk = 32 << 10

	// Ask for a chunk far larger than the server allows.
	e := box.engine(Options{Overwrite: true, ChunkSize: 16 << 20, PutThreshold: 1 << 10})
	body := payload(128 << 10)
	local := writeLocal(t, t.TempDir(), "big.bin", body)

	if _, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/big.bin"); err != nil {
		t.Fatal(err)
	}
	if box.patchCount != 4 {
		t.Errorf("PATCH count = %d, want 4 chunks of the server's 32 KiB limit", box.patchCount)
	}
}

// TestUploadResumesAfterInterruption is the property resumable uploads exist
// for: a transfer that dies partway must continue, not start again.
func TestUploadResumesAfterInterruption(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	box.maxChunk = 64 << 10
	box.failChunkAfter = 100 << 10

	stateDir := t.TempDir()
	body := payload(1 << 20)
	local := writeLocal(t, t.TempDir(), "big.bin", body)
	remote := "/eos/user/e/einstein/big.bin"

	first := box.engine(Options{Overwrite: true, ChunkSize: 64 << 10, PutThreshold: 1 << 10, StateDir: stateDir})
	if _, err := first.UploadFile(context.Background(), local, remote); err == nil {
		t.Fatal("expected the first attempt to fail")
	}
	if _, ok := box.fileContent(remote); ok {
		t.Fatal("a partial upload should not have produced a complete file")
	}

	patchesBefore := box.patchCount
	box.failChunkAfter = 0

	second := box.engine(Options{Overwrite: true, ChunkSize: 64 << 10, PutThreshold: 1 << 10, StateDir: stateDir})
	if _, err := second.UploadFile(context.Background(), local, remote); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	box.requireContent(remote, body)

	// One upload session, not two: resume reused the server-side upload.
	if box.tusPostCount != 1 {
		t.Errorf("TUS creations = %d, want 1: the second run restarted the upload", box.tusPostCount)
	}
	// And it did not re-send what the server already had.
	resendPatches := box.patchCount - patchesBefore
	if resendPatches >= 16 {
		t.Errorf("the resume sent %d chunks, want roughly the remaining ~14", resendPatches)
	}
}

// TestUploadDoesNotResumeAfterLocalEdit: splicing two different versions of a
// file together would pass every length check while being silently wrong.
func TestUploadDoesNotResumeAfterLocalEdit(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	box.maxChunk = 64 << 10
	box.failChunkAfter = 100 << 10

	stateDir := t.TempDir()
	localDir := t.TempDir()
	local := writeLocal(t, localDir, "big.bin", payload(1<<20))
	remote := "/eos/user/e/einstein/big.bin"

	first := box.engine(Options{Overwrite: true, ChunkSize: 64 << 10, PutThreshold: 1 << 10, StateDir: stateDir})
	if _, err := first.UploadFile(context.Background(), local, remote); err == nil {
		t.Fatal("expected the first attempt to fail")
	}

	// Replace the file with different content of a different size.
	replacement := bytes.Repeat([]byte("Z"), 2<<20)
	writeLocal(t, localDir, "big.bin", replacement)
	box.failChunkAfter = 0

	second := box.engine(Options{Overwrite: true, ChunkSize: 64 << 10, PutThreshold: 1 << 10, StateDir: stateDir})
	if _, err := second.UploadFile(context.Background(), local, remote); err != nil {
		t.Fatal(err)
	}
	box.requireContent(remote, replacement)

	if box.tusPostCount != 2 {
		t.Errorf("TUS creations = %d, want 2: an edited file must start a fresh upload", box.tusPostCount)
	}
}

func TestUploadSkipsExistingWithoutOverwrite(t *testing.T) {
	box := newFakeBox(t)
	box.putFile("/eos/user/e/einstein/notes.txt", []byte("original"))
	e := box.engine(Options{})

	local := writeLocal(t, t.TempDir(), "notes.txt", []byte("replacement"))
	_, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt")
	if !errors.Is(err, errSkipped) {
		t.Fatalf("got %v, want the upload to be skipped", err)
	}
	box.requireContent("/eos/user/e/einstein/notes.txt", []byte("original"))
}

func TestUploadOverwrites(t *testing.T) {
	box := newFakeBox(t)
	box.putFile("/eos/user/e/einstein/notes.txt", []byte("original"))
	e := box.engine(Options{Overwrite: true})

	local := writeLocal(t, t.TempDir(), "notes.txt", []byte("replacement"))
	if _, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt"); err != nil {
		t.Fatal(err)
	}
	box.requireContent("/eos/user/e/einstein/notes.txt", []byte("replacement"))
}

func TestUploadDryRunTransfersNothing(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	e := box.engine(Options{DryRun: true, Overwrite: true})

	local := writeLocal(t, t.TempDir(), "notes.txt", []byte("hello"))
	n, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("dry run reported %d bytes, want the size it would have sent", n)
	}
	if _, ok := box.fileContent("/eos/user/e/einstein/notes.txt"); ok {
		t.Error("a dry run wrote a file")
	}
	if box.putCount != 0 || box.tusPostCount != 0 {
		t.Error("a dry run issued write requests")
	}
}

func TestUploadSendsChecksumWhenVerifying(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	e := box.engine(Options{Overwrite: true, Verify: true})

	body := []byte("verify me")
	local := writeLocal(t, t.TempDir(), "notes.txt", body)
	if _, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt"); err != nil {
		t.Fatal(err)
	}

	got, err := e.fileChecksum(context.Background(), local)
	if err != nil {
		t.Fatal(err)
	}
	want := "MD5:" + md5hex(body)
	if got != want {
		t.Errorf("checksum = %q, want %q in the algorithm the server prefers", got, want)
	}
}

func TestUploadRejectsDirectoryWithoutRecursion(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{})
	dir := t.TempDir()

	_, err := e.UploadFile(context.Background(), dir, "/eos/user/e/einstein/x")
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error suggesting -r", err)
	}
	if !strings.Contains(err.Error(), "-r") {
		t.Errorf("the error should mention -r, got %q", err)
	}
}

func TestUploadTree(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true, Jobs: 4})

	local := t.TempDir()
	writeLocal(t, local, "a.txt", []byte("a"))
	writeLocal(t, local, "sub/b.txt", []byte("bb"))
	writeLocal(t, local, "sub/deep/c.txt", []byte("ccc"))

	stats, err := e.UploadTree(context.Background(), local, "/eos/user/e/einstein/data")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 3 {
		t.Errorf("Files = %d, want 3", stats.Files)
	}
	if stats.Bytes != 6 {
		t.Errorf("Bytes = %d, want 6", stats.Bytes)
	}

	box.requireContent("/eos/user/e/einstein/data/a.txt", []byte("a"))
	box.requireContent("/eos/user/e/einstein/data/sub/b.txt", []byte("bb"))
	box.requireContent("/eos/user/e/einstein/data/sub/deep/c.txt", []byte("ccc"))

	// Directories must exist before their files, or every upload into them
	// would fail with a conflict.
	for _, dir := range []string{
		"/eos/user/e/einstein/data",
		"/eos/user/e/einstein/data/sub",
		"/eos/user/e/einstein/data/sub/deep",
	} {
		if !box.hasDir(dir) {
			t.Errorf("directory %s was not created", dir)
		}
	}
}

func TestUploadTreeSkipsSymlinks(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Overwrite: true})

	local := t.TempDir()
	writeLocal(t, local, "real.txt", []byte("real"))
	if err := os.Symlink(filepath.Join(local, "real.txt"), filepath.Join(local, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	stats, err := e.UploadTree(context.Background(), local, "/eos/user/e/einstein/data")
	if err != nil {
		t.Fatal(err)
	}
	// Copying the target under the link's name would silently change the shape
	// of what the user asked to transfer.
	if stats.Files != 1 {
		t.Errorf("Files = %d, want 1: the symlink should have been skipped", stats.Files)
	}
	if _, ok := box.fileContent("/eos/user/e/einstein/data/link.txt"); ok {
		t.Error("the symlink was uploaded")
	}
}

func TestUploadTreeSingleFile(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")
	e := box.engine(Options{Overwrite: true})

	local := writeLocal(t, t.TempDir(), "a.txt", []byte("a"))
	stats, err := e.UploadTree(context.Background(), local, "/eos/user/e/einstein/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Bytes != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

// -- download -----------------------------------------------------------------

func TestDownloadFile(t *testing.T) {
	box := newFakeBox(t)
	body := []byte("downloaded content")
	box.putFile("/eos/user/e/einstein/notes.txt", body)
	e := box.engine(Options{Overwrite: true})

	local := filepath.Join(t.TempDir(), "notes.txt")
	n, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/notes.txt", local)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)) {
		t.Errorf("downloaded %d bytes, want %d", n, len(body))
	}
	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("content = %q, want %q", got, body)
	}
}

// TestDownloadUsesPartFile: a consumer watching the destination must never see
// a truncated file, and an interrupted download must leave something to resume.
func TestDownloadUsesPartFile(t *testing.T) {
	box := newFakeBox(t)
	body := payload(1 << 16)
	box.putFile("/eos/user/e/einstein/big.bin", body)

	dir := t.TempDir()
	local := filepath.Join(dir, "big.bin")

	// Pre-seed a partial download.
	if err := os.WriteFile(local+".part", body[:1000], 0o644); err != nil {
		t.Fatal(err)
	}

	e := box.engine(Options{Overwrite: true})
	n, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/big.bin", local)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)-1000) {
		t.Errorf("downloaded %d bytes, want only the remaining %d", n, len(body)-1000)
	}

	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Error("the resumed file does not match the source")
	}
	if _, err := os.Stat(local + ".part"); !os.IsNotExist(err) {
		t.Error("the .part file was left behind")
	}
}

func TestDownloadSkipsExistingWithoutOverwrite(t *testing.T) {
	box := newFakeBox(t)
	box.putFile("/eos/user/e/einstein/notes.txt", []byte("remote"))
	e := box.engine(Options{})

	dir := t.TempDir()
	local := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(local, []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/notes.txt", local)
	if !errors.Is(err, errSkipped) {
		t.Fatalf("got %v, want the download to be skipped", err)
	}
	got, _ := os.ReadFile(local)
	if string(got) != "local" {
		t.Errorf("the local file was overwritten: %q", got)
	}
}

// TestDownloadDetectsCorruption is what --verify is for: a truncated or
// corrupted transfer that looks complete is worse than one that fails.
func TestDownloadDetectsCorruption(t *testing.T) {
	box := newFakeBox(t)
	box.putFile("/eos/user/e/einstein/notes.txt", []byte("the real content"))
	box.corruptDownload = true

	e := box.engine(Options{Overwrite: true, Verify: true})
	local := filepath.Join(t.TempDir(), "notes.txt")

	_, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/notes.txt", local)
	if err == nil {
		t.Fatal("a corrupted download should have been rejected")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("got %v, want a checksum mismatch", err)
	}
}

func TestDownloadWithoutVerifyIgnoresChecksum(t *testing.T) {
	box := newFakeBox(t)
	box.putFile("/eos/user/e/einstein/notes.txt", []byte("the real content"))
	box.corruptDownload = true

	e := box.engine(Options{Overwrite: true})
	local := filepath.Join(t.TempDir(), "notes.txt")
	if _, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/notes.txt", local); err != nil {
		t.Fatalf("without --verify the download should succeed: %v", err)
	}
}

func TestDownloadTreeViaArchiver(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein/data/sub")
	box.putFile("/eos/user/e/einstein/data/a.txt", []byte("a"))
	box.putFile("/eos/user/e/einstein/data/sub/b.txt", []byte("bb"))

	e := box.engine(Options{Overwrite: true, Archive: true})
	local := t.TempDir()

	stats, err := e.DownloadTree(context.Background(), "/eos/user/e/einstein/data", local)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 {
		t.Errorf("Files = %d, want 2", stats.Files)
	}
	if box.archiveCount != 1 {
		t.Errorf("archiver requests = %d, want 1", box.archiveCount)
	}
	// One request for the whole tree, rather than one GET per file.
	if box.getCount != 0 {
		t.Errorf("individual GETs = %d, want 0 when the archiver is used", box.getCount)
	}

	// The same layout the walk produces: whether the archiver was used is a
	// transport detail, and a download that lands a level deeper depending on
	// which path the client took would be a trap for any script using it.
	assertFile(t, filepath.Join(local, "a.txt"), "a")
	assertFile(t, filepath.Join(local, "sub", "b.txt"), "bb")
}

// TestDownloadTreeFallsBackWhenArchiverIsAbsent: an unavailable archiver must
// not fail the command.
func TestDownloadTreeFallsBackWhenArchiverIsAbsent(t *testing.T) {
	box := newFakeBox(t)
	box.archiverEnabled = false
	box.mkdir("/eos/user/e/einstein/data")
	box.putFile("/eos/user/e/einstein/data/a.txt", []byte("a"))
	box.putFile("/eos/user/e/einstein/data/b.txt", []byte("bb"))

	e := box.engine(Options{Overwrite: true, Archive: true})
	local := t.TempDir()

	stats, err := e.DownloadTree(context.Background(), "/eos/user/e/einstein/data", local)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 {
		t.Errorf("Files = %d, want 2", stats.Files)
	}
	if box.getCount < 2 {
		t.Errorf("individual GETs = %d, want the walk fallback to have been used", box.getCount)
	}
	assertFile(t, filepath.Join(local, "a.txt"), "a")
	assertFile(t, filepath.Join(local, "b.txt"), "bb")
}

func TestDownloadTreeWalk(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein/data/sub")
	box.putFile("/eos/user/e/einstein/data/a.txt", []byte("a"))
	box.putFile("/eos/user/e/einstein/data/sub/b.txt", []byte("bb"))

	e := box.engine(Options{Overwrite: true, Jobs: 2})
	local := t.TempDir()

	stats, err := e.DownloadTree(context.Background(), "/eos/user/e/einstein/data", local)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Dirs != 2 {
		t.Errorf("stats = %+v, want 2 files and 2 directories", stats)
	}
	assertFile(t, filepath.Join(local, "a.txt"), "a")
	assertFile(t, filepath.Join(local, "sub", "b.txt"), "bb")
}

func TestDownloadRejectsDirectoryWithoutRecursion(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein/data")
	e := box.engine(Options{})

	_, err := e.DownloadFile(context.Background(), "/eos/user/e/einstein/data", filepath.Join(t.TempDir(), "x"))
	if cberr.KindOf(err) != cberr.KindUsage {
		t.Errorf("got %v, want a usage error", err)
	}
}

// -- concurrency --------------------------------------------------------------

func TestEachParallelRespectsJobLimit(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Jobs: 3})

	var mu sync.Mutex
	inFlight, peak := 0, 0

	err := e.eachParallel(context.Background(), 20, func(ctx context.Context, i int) error {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()

		mu.Lock()
		inFlight--
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if peak > 3 {
		t.Errorf("peak concurrency = %d, want at most the configured 3", peak)
	}
}

func TestEachParallelStopsOnFirstError(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Jobs: 2})

	sentinel := errors.New("boom")
	var mu sync.Mutex
	started := 0

	err := e.eachParallel(context.Background(), 100, func(ctx context.Context, i int) error {
		mu.Lock()
		started++
		mu.Unlock()
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("got %v, want the worker's error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if started >= 100 {
		t.Errorf("%d of 100 items started: the run should stop early on error", started)
	}
}

func TestEachParallelWithNoItems(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{Jobs: 4})
	if err := e.eachParallel(context.Background(), 0, func(context.Context, int) error {
		t.Fatal("fn should not be called")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// -- local walk ---------------------------------------------------------------

func TestWalkLocalOrdersDirectoriesShallowestFirst(t *testing.T) {
	root := t.TempDir()
	writeLocal(t, root, "a/b/c/deep.txt", []byte("x"))
	writeLocal(t, root, "a/shallow.txt", []byte("y"))

	dirs, files, err := walkLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", filepath.Join("a", "b"), filepath.Join("a", "b", "c")}
	if len(dirs) != len(want) {
		t.Fatalf("dirs = %v, want %v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Fatalf("dirs = %v, want shallowest first: %v", dirs, want)
		}
	}
	if len(files) != 2 {
		t.Errorf("files = %v, want 2", files)
	}
}

func TestOptionDefaults(t *testing.T) {
	box := newFakeBox(t)
	e := box.engine(Options{})
	opts := e.Options()

	if opts.Jobs < 2 {
		t.Errorf("Jobs = %d, want a sensible default", opts.Jobs)
	}
	if opts.ChunkSize != defaultChunkSize {
		t.Errorf("ChunkSize = %d, want the default", opts.ChunkSize)
	}
	if opts.PutThreshold != defaultPutThreshold {
		t.Errorf("PutThreshold = %d, want the default", opts.PutThreshold)
	}
	if opts.StateDir == "" {
		t.Error("StateDir should default to somewhere writable")
	}
}

func TestProgressEvents(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein")

	var mu sync.Mutex
	var events []Event
	e := box.engine(Options{Overwrite: true, Progress: func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}})

	local := writeLocal(t, t.TempDir(), "notes.txt", []byte("hello"))
	if _, err := e.UploadFile(context.Background(), local, "/eos/user/e/einstein/notes.txt"); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("no progress events were emitted")
	}
	last := events[len(events)-1]
	if !last.Done {
		t.Error("the final event should be marked done")
	}
	if last.Transferred != 5 || last.Total != 5 {
		t.Errorf("final event = %+v, want 5 of 5 bytes", last)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}
