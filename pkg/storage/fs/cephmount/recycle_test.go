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

package cephmount

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/pkg/xattr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRecycleTestFS also returns today's folder in the recycle bin of the space "proj".
func newRecycleTestFS(t *testing.T, spaceDepth int) (*cephmountfs, context.Context, string) {
	t.Helper()

	tempDir, cleanup := GetTestDir(t, "recycle")
	t.Cleanup(cleanup)

	ctx := appctx.ContextSetUser(ContextWithTestLogger(t), GetCurrentTestUser(t))
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{
		"testing_allow_local_mode": true,
		"enable_recycle":           true,
		"space_depth":              spaceDepth,
	}, "/volumes/_nogroup/test", tempDir)

	day := filepath.Join(tempDir, recycleDir, "proj", time.Now().UTC().Format("2006/01/02"))
	return fs, ctx, day
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestDeleteMovesFileToRecycle(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	writeTestFile(t, filepath.Join(fs.chrootDir, "proj", "a", "b", "file.txt"), "first")

	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a/b/file.txt"}))

	assert.NoFileExists(t, filepath.Join(fs.chrootDir, "proj", "a", "b", "file.txt"))
	content, err := os.ReadFile(filepath.Join(day, "a", "b", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first", string(content))

	for p := filepath.Join(day, "a", "b"); p != fs.chrootDir; p = filepath.Dir(p) {
		info, err := os.Stat(p)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(recycleDirPerms), info.Mode().Perm(), p)
	}
}

func TestDeleteSamePathTwiceKeepsBoth(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	file := filepath.Join(fs.chrootDir, "proj", "file.txt")
	ref := &provider.Reference{Path: "/proj/file.txt"}

	writeTestFile(t, file, "first")
	require.NoError(t, fs.Delete(ctx, ref))

	writeTestFile(t, file, "second")
	info, err := os.Stat(file)
	require.NoError(t, err)
	inode := strconv.FormatUint(info.Sys().(*syscall.Stat_t).Ino, 10)
	require.NoError(t, fs.Delete(ctx, ref))

	first, err := os.ReadFile(filepath.Join(day, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first", string(first))
	second, err := os.ReadFile(filepath.Join(day, "file.txt."+inode))
	require.NoError(t, err)
	assert.Equal(t, "second", string(second))
}

func TestDeleteFolderMergesIntoRecycle(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "a", "b", "one.txt"), "one")
	writeTestFile(t, filepath.Join(space, "a", "b", "two.txt"), "two")
	writeTestFile(t, filepath.Join(space, "a", "c", "three.txt"), "three")

	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a/b/one.txt"}))
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))

	assert.NoDirExists(t, filepath.Join(space, "a"))
	for _, f := range []string{"b/one.txt", "b/two.txt", "c/three.txt"} {
		assert.FileExists(t, filepath.Join(day, "a", f))
	}
}

func TestDeleteSpaceRootIsRefused(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	require.NoError(t, os.Mkdir(filepath.Join(fs.chrootDir, "proj"), 0755))

	err := fs.Delete(ctx, &provider.Reference{Path: "/proj"})

	assert.IsType(t, errtypes.PermissionDenied(""), err)
	assert.DirExists(t, filepath.Join(fs.chrootDir, "proj"))
}

func TestRecycleIsNotReachableByPath(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	writeTestFile(t, filepath.Join(fs.chrootDir, "proj", "deleted.txt"), "x")
	writeTestFile(t, filepath.Join(fs.chrootDir, "proj", "kept.txt"), "x")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/deleted.txt"}))
	binDay, err := filepath.Rel(fs.chrootDir, day)
	require.NoError(t, err)
	inBin := func(name string) *provider.Reference {
		return &provider.Reference{Path: "/" + filepath.Join(binDay, name)}
	}

	_, err = fs.GetMD(ctx, inBin("deleted.txt"), nil)
	assert.Error(t, err)
	_, err = fs.ListFolder(ctx, &provider.Reference{Path: "/proj/../" + recycleDir}, nil)
	assert.Error(t, err)

	assert.Error(t, fs.Move(ctx, &provider.Reference{Path: "/proj/kept.txt"}, inBin("kept.txt")))
	assert.FileExists(t, filepath.Join(fs.chrootDir, "proj", "kept.txt"))

	assert.Error(t, fs.CreateDir(ctx, inBin("new")))
	assert.NoDirExists(t, filepath.Join(day, "new"))
}

func TestListFolderHidesRecycleAtRoot(t *testing.T) {
	// With no space depth the whole mount is one space, thus the bin is next to its content.
	fs, ctx, _ := newRecycleTestFS(t, 0)
	writeTestFile(t, filepath.Join(fs.chrootDir, "deleted.txt"), "x")
	writeTestFile(t, filepath.Join(fs.chrootDir, "kept", recycleDir, "file.txt"), "x")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/deleted.txt"}))

	root, err := fs.ListFolder(ctx, &provider.Reference{Path: "/"}, nil)
	require.NoError(t, err)
	require.Len(t, root, 1)
	assert.Equal(t, "kept", root[0].Name)

	sub, err := fs.ListFolder(ctx, &provider.Reference{Path: "/kept"}, nil)
	require.NoError(t, err)
	require.Len(t, sub, 1)
	assert.Equal(t, recycleDir, sub[0].Name)
}

func recycleTimestamp(year int, month time.Month, day int) *typepb.Timestamp {
	return &typepb.Timestamp{Seconds: uint64(time.Date(year, month, day, 12, 0, 0, 0, time.UTC).Unix())}
}

func recycleItemsByKey(items []*provider.RecycleItem) map[string]*provider.RecycleItem {
	byKey := map[string]*provider.RecycleItem{}
	for _, item := range items {
		byKey[item.Key] = item
	}
	return byKey
}

func TestListRecycleMergesFoldersOverDays(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	bin := filepath.Join(fs.chrootDir, recycleDir, "proj")
	require.NoError(t, os.Mkdir(filepath.Join(fs.chrootDir, "proj"), 0755))
	writeTestFile(t, filepath.Join(bin, "2026", "11", "20", "a", "one.txt"), "one")
	writeTestFile(t, filepath.Join(bin, "2026", "11", "23", "a", "two.txt"), "two")
	writeTestFile(t, filepath.Join(bin, "2026", "11", "23", "top.txt"), "top")
	writeTestFile(t, filepath.Join(bin, "2026", "11", "25", "later.txt"), "later")

	root, err := fs.ListRecycle(ctx, "/proj", "", "", recycleTimestamp(2026, 11, 20), recycleTimestamp(2026, 11, 23))
	require.NoError(t, err)
	byKey := recycleItemsByKey(root)
	require.Len(t, byKey, 2)
	require.Contains(t, byKey, "20261120-20261123/a")
	assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, byKey["20261120-20261123/a"].Type)
	assert.Equal(t, "/proj/a", byKey["20261120-20261123/a"].Ref.Path)
	assert.Equal(t, uint64(len("one")+len("two")), byKey["20261120-20261123/a"].Size)
	require.Contains(t, byKey, "20261123/top.txt")
	assert.Equal(t, "/proj/top.txt", byKey["20261123/top.txt"].Ref.Path)

	// The storage provider splits the key at its first slash.
	sub, err := fs.ListRecycle(ctx, "/proj", "20261120-20261123", "/a", nil, nil)
	require.NoError(t, err)
	byKey = recycleItemsByKey(sub)
	require.Len(t, byKey, 2)
	assert.Equal(t, "/proj/a/one.txt", byKey["20261120/a/one.txt"].GetRef().GetPath())
	assert.Equal(t, "/proj/a/two.txt", byKey["20261123/a/two.txt"].GetRef().GetPath())

	oneDay, err := fs.ListRecycle(ctx, "/proj", "20261123", "/a", nil, nil)
	require.NoError(t, err)
	require.Len(t, oneDay, 1)
	assert.Equal(t, "20261123/a/two.txt", oneDay[0].Key)
}

func TestListRecycleShowsOriginalNames(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "file.txt"), "first")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/file.txt"}))
	writeTestFile(t, filepath.Join(space, "file.txt"), "second")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/file.txt"}))
	writeTestFile(t, filepath.Join(space, "report.2024"), "report")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/report.2024"}))
	// A folder that collides with a deleted file gets the suffix too.
	writeTestFile(t, filepath.Join(space, "a"), "file")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "x")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))

	// Without dates, today is listed.
	items, err := fs.ListRecycle(ctx, "/proj", "", "", nil, nil)
	require.NoError(t, err)
	var paths []string
	var folderKey string
	for _, item := range items {
		paths = append(paths, item.Ref.Path)
		if item.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
			folderKey = item.Key
		}
	}
	assert.ElementsMatch(t, []string{"/proj/file.txt", "/proj/file.txt", "/proj/report.2024", "/proj/a", "/proj/a"}, paths)
	assert.Len(t, recycleItemsByKey(items), 5, "each deletion has its own key")

	scope, rel, _ := strings.Cut(folderKey, "/")
	sub, err := fs.ListRecycle(ctx, "/proj", scope, "/"+rel, nil, nil)
	require.NoError(t, err)
	require.Len(t, sub, 1)
	assert.Equal(t, "/proj/a/x.txt", sub[0].Ref.Path)
}

func TestListRecycleMarksFoldersAsBrowsable(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "x")
	writeTestFile(t, filepath.Join(space, "top.txt"), "top")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/top.txt"}))

	items, err := fs.ListRecycle(ctx, "/proj", "", "", nil, nil)
	require.NoError(t, err)
	browsable := map[string]bool{}
	for _, item := range items {
		_, browsable[item.Ref.Path] = item.GetOpaque().GetMap()[storage.RecycleItemBrowsable]
	}
	assert.Equal(t, map[string]bool{"/proj/a": true, "/proj/top.txt": false}, browsable)
}

func TestListRecycleRefusals(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	require.NoError(t, os.MkdirAll(filepath.Join(space, "sub"), 0755))

	_, err := fs.ListRecycle(ctx, "/proj/sub", "", "", nil, nil)
	assert.IsType(t, errtypes.BadRequest(""), err, "not a space root")

	for _, key := range []string{"2026-11-23/a", "20261123-20261120", "20261123/../../x"} {
		_, err = fs.ListRecycle(ctx, "/proj", key, "", nil, nil)
		assert.IsType(t, errtypes.BadRequest(""), err, key)
	}

	require.NoError(t, os.Chmod(space, 0555))
	t.Cleanup(func() { _ = os.Chmod(space, 0755) })
	_, err = fs.ListRecycle(ctx, "/proj", "", "", nil, nil)
	assert.IsType(t, errtypes.PermissionDenied(""), err, "no write access to the space")
}

func TestListRecycleRootAsTheStorageProviderGivesIt(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	writeTestFile(t, filepath.Join(fs.chrootDir, "proj", "file.txt"), "deleted")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/file.txt"}))

	// router.ShiftPath splits an empty key into "" and "/".
	items, err := fs.ListRecycle(ctx, "/proj", "", "/", nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "/proj/file.txt", items[0].Ref.Path)
}

// splitRecycleKey splits a key as the storage provider does.
func splitRecycleKey(key string) (string, string) {
	scope, rel, _ := strings.Cut(key, "/")
	return scope, "/" + rel
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func TestRestoreRecycleItemRecreatesParents(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "a", "b", "file.txt"), "content")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))

	items, err := fs.ListRecycle(ctx, "/proj", "", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1)
	sub, err := fs.ListRecycle(ctx, "/proj", items[0].Key+"/b", "", nil, nil)
	require.NoError(t, err)
	require.Len(t, sub, 1)

	scope, rel := splitRecycleKey(sub[0].Key)
	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", scope, rel, nil, false))

	assert.Equal(t, "content", readTestFile(t, filepath.Join(space, "a", "b", "file.txt")))
	assert.NoDirExists(t, filepath.Join(day, "a"), "the emptied bin folders are removed")
}

func TestRestoreRecycleItemNewestCopyWins(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	bin := filepath.Join(fs.chrootDir, recycleDir, "proj")
	require.NoError(t, os.Mkdir(space, 0755))
	writeTestFile(t, filepath.Join(bin, "2026", "11", "20", "a", "one.txt"), "old one")
	writeTestFile(t, filepath.Join(bin, "2026", "11", "20", "a", "two.txt"), "two")
	writeTestFile(t, filepath.Join(bin, "2026", "11", "23", "a", "one.txt"), "new one")

	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", "20261120-20261123", "/a", nil, false))

	assert.Equal(t, "new one", readTestFile(t, filepath.Join(space, "a", "one.txt")))
	assert.Equal(t, "two", readTestFile(t, filepath.Join(space, "a", "two.txt")))
	assert.Equal(t, "old one", readTestFile(t, filepath.Join(bin, "2026", "11", "20", "a", "one.txt")), "the older copy stays in the bin")
}

func TestRestoreRecycleItemReportsACollisionOnce(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	bin := filepath.Join(fs.chrootDir, recycleDir, "proj")
	writeTestFile(t, filepath.Join(fs.chrootDir, "proj", "a", "path.txt"), "already there")
	writeTestFile(t, filepath.Join(bin, "2026", "10", "10", "a", "path.txt"), "older")
	writeTestFile(t, filepath.Join(bin, "2026", "10", "13", "a", "path.txt"), "newer")

	err := fs.RestoreRecycleItem(ctx, "/proj", "20261010-20261013", "/a", nil, false)

	var collisions storage.RecycleCollisions
	require.ErrorAs(t, err, &collisions)
	assert.Equal(t, storage.RecycleCollisions{{Key: "20261013/a/path.txt", Path: "a/path.txt"}}, collisions, "once, as the newest copy")
}

func TestRestoreRecycleItemNewestCopyOfTheDayWins(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "d", "f.txt"), "first")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/d/f.txt"}))
	// The order comes from the ctime, which can have a coarse resolution.
	time.Sleep(20 * time.Millisecond)
	writeTestFile(t, filepath.Join(space, "d", "f.txt"), "second")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/d/f.txt"}))

	today := time.Now().UTC().Format(recycleKeyDay)
	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", today, "/d", nil, false))

	assert.Equal(t, "second", readTestFile(t, filepath.Join(space, "d", "f.txt")))
	items, err := fs.ListRecycle(ctx, "/proj", today, "/d", nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1, "the first copy stays in the bin")
}

func TestRestoreRecycleItemDeletedFolderGetsTheNewestCopy(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "d", "f.txt"), "first")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/d/f.txt"}))
	// The order comes from the ctime, which can have a coarse resolution.
	time.Sleep(20 * time.Millisecond)
	writeTestFile(t, filepath.Join(space, "d", "f.txt"), "second")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/d/f.txt"}))
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/d"}))

	today := time.Now().UTC().Format(recycleKeyDay)
	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", today, "/d", nil, false))

	entries, err := os.ReadDir(filepath.Join(space, "d"))
	require.NoError(t, err)
	require.Len(t, entries, 1, "only the newest copy comes back")
	assert.Equal(t, "second", readTestFile(t, filepath.Join(space, "d", "f.txt")))
	items, err := fs.ListRecycle(ctx, "/proj", today, "/d", nil, nil)
	require.NoError(t, err)
	require.Len(t, items, 1, "the first copy stays in the bin")
}

func TestRestoreRecycleItemDoesNotOverwrite(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "deleted x")
	writeTestFile(t, filepath.Join(space, "a", "y.txt"), "deleted y")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "new x")

	today := time.Now().UTC().Format(recycleKeyDay)
	err := fs.RestoreRecycleItem(ctx, "/proj", today, "/a", nil, false)

	var collisions storage.RecycleCollisions
	require.ErrorAs(t, err, &collisions)
	assert.Equal(t, storage.RecycleCollisions{{Key: today + "/a/x.txt", Path: "a/x.txt"}}, collisions)
	assert.Equal(t, "new x", readTestFile(t, filepath.Join(space, "a", "x.txt")))
	assert.Equal(t, "deleted y", readTestFile(t, filepath.Join(space, "a", "y.txt")))
	assert.Equal(t, "deleted x", readTestFile(t, filepath.Join(day, "a", "x.txt")), "the collision stays in the bin")
}

func TestRestoreRecycleItemWithForceKeepsBoth(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	space := filepath.Join(fs.chrootDir, "proj")
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "deleted x")
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a"}))
	writeTestFile(t, filepath.Join(space, "a", "x.txt"), "new x")
	info, err := os.Stat(filepath.Join(space, "a", "x.txt"))
	require.NoError(t, err)
	inode := strconv.FormatUint(info.Sys().(*syscall.Stat_t).Ino, 10)

	today := time.Now().UTC().Format(recycleKeyDay)
	var collisions storage.RecycleCollisions
	require.ErrorAs(t, fs.RestoreRecycleItem(ctx, "/proj", today, "/a", nil, false), &collisions)
	require.Len(t, collisions, 1)

	// The key of a collision restores that item alone.
	scope, rel := splitRecycleKey(collisions[0].Key)
	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", scope, rel, nil, true))

	assert.Equal(t, "deleted x", readTestFile(t, filepath.Join(space, "a", "x.txt")))
	assert.Equal(t, "new x", readTestFile(t, filepath.Join(space, "a", "x.txt."+inode)), "the item that was in the way")
}

func TestRestoreRecycleItemRefusals(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	require.NoError(t, os.Mkdir(filepath.Join(fs.chrootDir, "proj"), 0755))
	today := time.Now().UTC().Format(recycleKeyDay)

	assert.IsType(t, errtypes.BadRequest(""), fs.RestoreRecycleItem(ctx, "/proj", today, "", nil, false), "no item in the key")
	assert.IsType(t, errtypes.NotFound(""), fs.RestoreRecycleItem(ctx, "/proj", today, "/missing.txt", nil, false))
}

// setTestACL skips the test where the file system or the tools have no ACLs.
func setTestACL(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("setfacl", args...).CombinedOutput(); err != nil {
		t.Skipf("no ACL support: %v: %s", err, out)
	}
}

func testACL(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("getfacl", "--omit-header", "--numeric", path).CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func TestRestoreRecycleItemResetsGrants(t *testing.T) {
	fs, ctx, _ := newRecycleTestFS(t, 1)
	folder := filepath.Join(fs.chrootDir, "proj", "a")
	file := filepath.Join(folder, "f.txt")
	writeTestFile(t, file, "x")
	require.NoError(t, os.Chmod(file, 0640))
	setTestACL(t, "-m", "u:65533:rwx", file)
	if err := xattr.Set(file, xattrExtShare+testExternalAccount, []byte("rwx")); err != nil {
		t.Skipf("no user xattr support: %v", err)
	}
	require.NoError(t, xattr.Set(file, xattrLock, []byte("lock")))
	require.NoError(t, fs.Delete(ctx, &provider.Reference{Path: "/proj/a/f.txt"}))
	// The grants of the folder change while the file is in the bin.
	setTestACL(t, "-m", "d:g:65534:r-x", folder)

	today := time.Now().UTC().Format(recycleKeyDay)
	require.NoError(t, fs.RestoreRecycleItem(ctx, "/proj", today, "/a/f.txt", nil, false))

	acl := testACL(t, file)
	assert.NotContains(t, acl, "user:65533:", "the old grant does not come back")
	assert.Contains(t, acl, "group:65534:r-x", "the grant of the folder applies")
	assert.Contains(t, acl, "user::rw-")
	assert.Contains(t, acl, "other::---", "the mode bits stay")
	names, err := xattr.List(file)
	require.NoError(t, err)
	assert.NotContains(t, names, xattrExtShare+testExternalAccount, "the external grant does not come back")
	assert.NotContains(t, names, xattrLock)
}

func TestRecycleDoesNotFollowSymlinks(t *testing.T) {
	fs, ctx, day := newRecycleTestFS(t, 1)
	require.NoError(t, os.Mkdir(filepath.Join(fs.chrootDir, "proj"), 0755))
	secret := filepath.Join(fs.chrootDir, "other", "secret.txt")
	writeTestFile(t, secret, "secret")
	setTestACL(t, "-m", "u:65533:rwx", secret)

	// A deleted symlink that leads from the bin to another space.
	require.NoError(t, os.MkdirAll(day, 0755))
	up, err := filepath.Rel(day, fs.chrootDir)
	require.NoError(t, err)
	require.NoError(t, os.Symlink(filepath.Join(up, "other"), filepath.Join(day, "a")))
	today := time.Now().UTC().Format(recycleKeyDay)

	items, err := fs.ListRecycle(ctx, "/proj", today, "/a", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, items)

	err = fs.RestoreRecycleItem(ctx, "/proj", today, "/a/secret.txt", nil, false)
	assert.IsType(t, errtypes.NotFound(""), err)
	assert.FileExists(t, secret)
	assert.Contains(t, testACL(t, secret), "user:65533:rwx")
}
