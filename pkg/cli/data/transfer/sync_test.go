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
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// syncFixture sets up a fake the server and a local tree for one sync test.
type syncFixture struct {
	t     *testing.T
	box   *fakeBox
	e     *Engine
	local string
}

func newSyncFixture(t *testing.T, opts Options) *syncFixture {
	t.Helper()
	box := newFakeBox(t)
	if opts.Jobs == 0 {
		opts.Jobs = 2
	}
	return &syncFixture{t: t, box: box, e: box.engine(opts), local: t.TempDir()}
}

func (f *syncFixture) writeLocal(rel, body string) {
	f.t.Helper()
	writeLocal(f.t, f.local, rel, []byte(body))
}

// touchLocal sets an mtime, so a test can express "the local copy is newer".
func (f *syncFixture) touchLocal(rel string, at time.Time) {
	f.t.Helper()
	p := filepath.Join(f.local, filepath.FromSlash(rel))
	if err := os.Chtimes(p, at, at); err != nil {
		f.t.Fatal(err)
	}
}

func (f *syncFixture) localExists(rel string) bool {
	_, err := os.Lstat(filepath.Join(f.local, filepath.FromSlash(rel)))
	return err == nil
}

func (f *syncFixture) readLocal(rel string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.local, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatalf("reading %s: %v", rel, err)
	}
	return string(b)
}

const syncRemote = "/eos/user/e/einstein/data"

func (f *syncFixture) push(opts SyncOptions) *SyncStats {
	f.t.Helper()
	opts.Direction = Push
	stats, err := f.e.Sync(context.Background(), f.local, syncRemote, opts)
	if err != nil {
		f.t.Fatalf("push: %v", err)
	}
	return stats
}

func (f *syncFixture) pull(opts SyncOptions) *SyncStats {
	f.t.Helper()
	opts.Direction = Pull
	stats, err := f.e.Sync(context.Background(), f.local, syncRemote, opts)
	if err != nil {
		f.t.Fatalf("pull: %v", err)
	}
	return stats
}

// -- push ---------------------------------------------------------------------

func TestSyncPushCreatesTree(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")
	f.writeLocal("sub/b.txt", "beta")

	stats := f.push(SyncOptions{})
	if stats.Created != 2 {
		t.Errorf("Created = %d, want 2", stats.Created)
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("alpha"))
	f.box.requireContent(syncRemote+"/sub/b.txt", []byte("beta"))
	if !f.box.hasDir(syncRemote + "/sub") {
		t.Error("the subdirectory was not created")
	}
}

// TestSyncPushSkipsUnchanged is the property that makes sync usable on a large
// tree: a second run must move nothing.
func TestSyncPushSkipsUnchanged(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")
	f.writeLocal("sub/b.txt", "beta")

	f.push(SyncOptions{})
	putsAfterFirst := f.box.putCount

	stats := f.push(SyncOptions{})
	if stats.Created != 0 || stats.Updated != 0 {
		t.Errorf("a second run transferred files: %+v", stats)
	}
	if stats.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", stats.Skipped)
	}
	if f.box.putCount != putsAfterFirst {
		t.Errorf("the second run issued %d more uploads, want 0", f.box.putCount-putsAfterFirst)
	}
}

func TestSyncPushUpdatesChangedFile(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")
	f.push(SyncOptions{})

	// Different size: unambiguously changed.
	f.writeLocal("a.txt", "alpha and more")
	stats := f.push(SyncOptions{})

	if stats.Updated != 1 {
		t.Errorf("Updated = %d, want 1: %+v", stats.Updated, stats)
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("alpha and more"))
}

// TestSyncPushUpdatesOnNewerMtime covers the same-size edit, where only the
// timestamp distinguishes the two.
func TestSyncPushUpdatesOnNewerMtime(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")
	f.push(SyncOptions{})

	f.writeLocal("a.txt", "AAAAA")
	f.touchLocal("a.txt", time.Now().Add(time.Hour))

	stats := f.push(SyncOptions{})
	if stats.Updated != 1 {
		t.Errorf("Updated = %d, want 1 for a same-size but newer file", stats.Updated)
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("AAAAA"))
}

// TestSyncPushWithoutDeleteKeepsExtras: without --delete, sync only adds and
// updates, so a file that exists only on the destination survives.
func TestSyncPushWithoutDeleteKeepsExtras(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/orphan.txt", []byte("keep me"))
	f.writeLocal("a.txt", "alpha")

	stats := f.push(SyncOptions{})
	if stats.Deleted != 0 {
		t.Errorf("Deleted = %d, want 0 without --delete", stats.Deleted)
	}
	if _, ok := f.box.fileContent(syncRemote + "/orphan.txt"); !ok {
		t.Error("an extra destination file was deleted without --delete")
	}
}

func TestSyncPushWithDeleteRemovesExtras(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/orphan.txt", []byte("remove me"))
	f.writeLocal("a.txt", "alpha")

	stats := f.push(SyncOptions{Delete: true})
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if _, ok := f.box.fileContent(syncRemote + "/orphan.txt"); ok {
		t.Error("--delete did not remove the extra file")
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("alpha"))
}

func TestSyncPushDryRunChangesNothing(t *testing.T) {
	f := newSyncFixture(t, Options{DryRun: true})
	f.box.putFile(syncRemote+"/orphan.txt", []byte("still here"))
	f.writeLocal("a.txt", "alpha")

	stats := f.push(SyncOptions{Delete: true})
	if stats.Created != 1 || stats.Deleted != 1 {
		t.Errorf("a dry run should still report the plan: %+v", stats)
	}
	if _, ok := f.box.fileContent(syncRemote + "/a.txt"); ok {
		t.Error("a dry run uploaded a file")
	}
	if _, ok := f.box.fileContent(syncRemote + "/orphan.txt"); !ok {
		t.Error("a dry run deleted a file")
	}
}

func TestSyncSkipsHiddenByDefault(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")
	f.writeLocal(".hidden", "secret")
	f.writeLocal(".config/settings", "secret")

	f.push(SyncOptions{})
	if _, ok := f.box.fileContent(syncRemote + "/.hidden"); ok {
		t.Error("a dot-file was synced without --hidden")
	}
	if _, ok := f.box.fileContent(syncRemote + "/.config/settings"); ok {
		t.Error("a file inside a dot-directory was synced without --hidden")
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("alpha"))
}

func TestSyncIncludesHiddenWhenAsked(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal(".hidden", "secret")

	f.push(SyncOptions{IncludeHidden: true})
	f.box.requireContent(syncRemote+"/.hidden", []byte("secret"))
}

// -- pull ---------------------------------------------------------------------

func TestSyncPullCreatesTree(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.mkdir(syncRemote + "/sub")
	f.box.putFile(syncRemote+"/a.txt", []byte("alpha"))
	f.box.putFile(syncRemote+"/sub/b.txt", []byte("beta"))

	stats := f.pull(SyncOptions{})
	if stats.Created != 2 {
		t.Errorf("Created = %d, want 2: %+v", stats.Created, stats)
	}
	if got := f.readLocal("a.txt"); got != "alpha" {
		t.Errorf("a.txt = %q", got)
	}
	if got := f.readLocal("sub/b.txt"); got != "beta" {
		t.Errorf("sub/b.txt = %q", got)
	}
}

func TestSyncPullWithDeleteRemovesLocalExtras(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/a.txt", []byte("alpha"))
	f.writeLocal("orphan.txt", "remove me")

	stats := f.pull(SyncOptions{Delete: true})
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if f.localExists("orphan.txt") {
		t.Error("--delete did not remove the local extra")
	}
	if got := f.readLocal("a.txt"); got != "alpha" {
		t.Errorf("a.txt = %q", got)
	}
}

func TestSyncPullSkipsUnchanged(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/a.txt", []byte("alpha"))

	f.pull(SyncOptions{})
	getsAfterFirst := f.box.getCount

	stats := f.pull(SyncOptions{})
	if stats.Created != 0 || stats.Updated != 0 {
		t.Errorf("a second pull transferred files: %+v", stats)
	}
	if f.box.getCount != getsAfterFirst {
		t.Errorf("the second pull issued %d more downloads, want 0", f.box.getCount-getsAfterFirst)
	}
}

// -- edge cases ---------------------------------------------------------------

// TestSyncPushToMissingRemoteRoot: a first push must work without a
// preparatory mkdir, so a missing root reads as an empty tree.
func TestSyncPushToMissingRemoteRoot(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("a.txt", "alpha")

	stats := f.push(SyncOptions{})
	if stats.Created != 1 {
		t.Errorf("Created = %d, want 1", stats.Created)
	}
	f.box.requireContent(syncRemote+"/a.txt", []byte("alpha"))
}

// TestSyncDoesNotReplaceDirectoryWithFile: replacing a directory means deleting
// a subtree, which a run without --delete must never do silently.
func TestSyncDoesNotReplaceDirectoryWithFile(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.mkdir(syncRemote + "/collision")
	f.box.putFile(syncRemote+"/collision/inside.txt", []byte("subtree"))
	f.writeLocal("collision", "now a file")

	stats := f.push(SyncOptions{})
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want the collision skipped: %+v", stats.Skipped, stats)
	}
	if _, ok := f.box.fileContent(syncRemote + "/collision/inside.txt"); !ok {
		t.Error("the remote subtree was destroyed by a name collision")
	}
}

func TestSyncRejectsNonDirectory(t *testing.T) {
	f := newSyncFixture(t, Options{})
	local := writeLocal(t, t.TempDir(), "file.txt", []byte("x"))

	_, err := f.e.Sync(context.Background(), local, syncRemote, SyncOptions{Direction: Push})
	if err == nil {
		t.Fatal("syncing a single file should be rejected")
	}
}

func TestSyncDeleteRemovesDeepestFirst(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.mkdir(syncRemote + "/gone/deeper")
	f.box.putFile(syncRemote+"/gone/deeper/x.txt", []byte("x"))
	f.writeLocal("keep.txt", "keep")

	stats := f.push(SyncOptions{Delete: true})
	if stats.Deleted == 0 {
		t.Fatal("nothing was deleted")
	}
	if _, ok := f.box.fileContent(syncRemote + "/gone/deeper/x.txt"); ok {
		t.Error("the nested file survived --delete")
	}
	if f.box.hasDir(syncRemote + "/gone") {
		t.Error("the directory survived --delete")
	}
}

func TestDirectionString(t *testing.T) {
	if Push.String() != "push" || Pull.String() != "pull" {
		t.Errorf("Direction.String is wrong: %q %q", Push, Pull)
	}
}

func TestHasHiddenComponent(t *testing.T) {
	tests := map[string]bool{
		"a.txt":            false,
		".hidden":          true,
		"sub/.hidden":      true,
		".config/settings": true,
		"sub/dir/file.txt": false,
	}
	for rel, want := range tests {
		if got := hasHiddenComponent(rel); got != want {
			t.Errorf("hasHiddenComponent(%q) = %v, want %v", rel, got, want)
		}
	}
}

// TestSyncPullCreatesTheDestination: taking a first copy of something is the
// ordinary reason to pull, and the destination does not exist yet then.
func TestSyncPullCreatesTheDestination(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein/data")
	box.putFile("/eos/user/e/einstein/data/a.txt", []byte("a"))

	e := box.engine(Options{Overwrite: true})
	dest := filepath.Join(t.TempDir(), "not-there-yet")

	stats, err := e.Sync(context.Background(), dest, "/eos/user/e/einstein/data",
		SyncOptions{Direction: Pull})
	if err != nil {
		t.Fatalf("pulling into a missing directory should create it: %v", err)
	}
	if stats.Created == 0 {
		t.Errorf("stats = %+v, want something created", stats)
	}
	assertFile(t, filepath.Join(dest, "a.txt"), "a")
}

// TestSyncPushStillRequiresTheSource: the same leniency would be wrong here,
// since an absent source would mirror an empty tree over the remote.
func TestSyncPushStillRequiresTheSource(t *testing.T) {
	box := newFakeBox(t)
	box.mkdir("/eos/user/e/einstein/data")

	e := box.engine(Options{Overwrite: true})
	missing := filepath.Join(t.TempDir(), "not-there-yet")

	if _, err := e.Sync(context.Background(), missing, "/eos/user/e/einstein/data",
		SyncOptions{Direction: Push}); err == nil {
		t.Error("pushing from a missing directory should fail, not create it and mirror nothing")
	}
}

// -- exclude ------------------------------------------------------------------

func TestExcludedMatching(t *testing.T) {
	for _, tc := range []struct {
		pattern, rel string
		want         bool
	}{
		// A pattern without a slash matches any component, at any depth.
		{"*.o", "main.o", true},
		{"*.o", "sub/main.o", true},
		{"*.o", "main.c", false},
		{"build", "build", true},
		{"build", "build/app", true},
		{"build", "sub/build/app", true},
		{"build", "rebuild", false},
		// A pattern with a slash matches the whole relative path.
		{"build/*", "build/app", true},
		{"build/*", "sub/build/app", false},
		{"build/*", "build", false},
		{"sub/*.o", "sub/main.o", true},
		// A malformed pattern excludes nothing rather than everything: the
		// alternative is a sync that silently copies no files at all.
		{"[", "anything", false},
	} {
		t.Run(tc.pattern+" vs "+tc.rel, func(t *testing.T) {
			opts := SyncOptions{Exclude: []string{tc.pattern}}
			if got := opts.Excluded(tc.rel); got != tc.want {
				t.Errorf("Excluded(%q) with --exclude %q = %v, want %v",
					tc.rel, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestSyncExcludeLeavesFilesOut(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.writeLocal("main.c", "source")
	f.writeLocal("main.o", "object")
	f.writeLocal("sub/other.o", "object")
	f.writeLocal("build/app", "binary")

	f.push(SyncOptions{Exclude: []string{"*.o", "build"}})

	f.box.requireContent(syncRemote+"/main.c", []byte("source"))
	for _, rel := range []string{"/main.o", "/sub/other.o", "/build/app"} {
		if _, ok := f.box.fileContent(syncRemote + rel); ok {
			t.Errorf("%s was synced despite being excluded", rel)
		}
	}
}

// TestSyncExcludeProtectsFromDelete is the property that makes --exclude safe
// to combine with --delete: an excluded entry is invisible on both sides, so it
// is neither copied nor removed. Were it invisible only on the source side,
// --delete would take it as an entry the source does not have.
func TestSyncExcludeProtectsFromDelete(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/notes.o", []byte("not mine to delete"))
	f.box.putFile(syncRemote+"/stale.txt", []byte("delete me"))
	f.writeLocal("main.c", "source")

	stats := f.push(SyncOptions{Delete: true, Exclude: []string{"*.o"}})

	if _, ok := f.box.fileContent(syncRemote + "/notes.o"); !ok {
		t.Error("--delete removed an excluded file")
	}
	if _, ok := f.box.fileContent(syncRemote + "/stale.txt"); ok {
		t.Error("--delete kept a file that is not in the source")
	}
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1: only the unexcluded extra", stats.Deleted)
	}
}

func TestSyncExcludeAppliesWhenPulling(t *testing.T) {
	f := newSyncFixture(t, Options{})
	f.box.putFile(syncRemote+"/a.txt", []byte("alpha"))
	f.box.putFile(syncRemote+"/scratch/big.tmp", []byte("temporary"))

	f.pull(SyncOptions{Exclude: []string{"*.tmp"}})

	if f.readLocal("a.txt") != "alpha" {
		t.Error("the unexcluded file was not pulled")
	}
	if f.localExists("scratch/big.tmp") {
		t.Error("an excluded file was pulled")
	}
}
