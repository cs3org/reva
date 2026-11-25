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
	"cmp"
	"context"
	iofs "io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typepb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/pkg/errors"
	"github.com/pkg/xattr"
	"golang.org/x/sys/unix"
)

const (
	// recycleDir is outside the spaces, so that deleted items do not count in their quota.
	recycleDir = ".recycle"
	// Everyone can move items in, but only root can list them.
	recycleDirPerms = 0o753
	// The day folders below the recycle bin of a space, in UTC.
	recycleDayDirs = "2006/01/02"
	// The first segment of a key: one day, or the first and last day of a listing.
	recycleKeyDay = "20060102"
)

// inRecycle reports whether a chroot-relative path is in the recycle bin.
func inRecycle(chrootPath string) bool {
	first, _, _ := strings.Cut(chrootPath, string(filepath.Separator))
	return first == recycleDir
}

// moveToRecycle moves path to .recycle/<space root>/YYYY/MM/DD/<path in the space>.
// The date is in UTC, so that Reva and the Samba VFS plugin use the same day folder.
func (fs *cephmountfs) moveToRecycle(ctx context.Context, path string) error {
	spaceRoot := "."
	if root, ok := fs.spaceRootFor(path); ok {
		spaceRoot = root
	}
	rel, err := filepath.Rel(spaceRoot, path)
	if err != nil {
		return err
	}
	target := filepath.Join(recycleDir, spaceRoot, time.Now().UTC().Format(recycleDayDirs), rel)
	if err := fs.createRecycleDirs(filepath.Dir(target)); err != nil {
		return errors.Wrap(err, "cephmount: failed to create the recycle bin folders")
	}

	// Not Stat: a symlink to a folder must be moved as a link, not merged.
	info, err := fs.rootFS.Lstat(path)
	if err != nil {
		return err
	}
	return fs.recycleEntry(ctx, path, info, target)
}

// createRecycleDirs creates the missing folders as root, so that a user who
// deletes an item first does not own a folder that others delete into.
func (fs *cephmountfs) createRecycleDirs(dir string) error {
	if _, err := fs.rootFS.Stat(dir); err == nil {
		return nil
	}
	p := ""
	for part := range strings.SplitSeq(dir, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		if err := fs.rootFS.Mkdir(p, recycleDirPerms); err != nil {
			if os.IsExist(err) {
				continue
			}
			return err
		}
		// Mkdir applies the umask, which removes the write bit for others.
		if err := fs.rootFS.Chmod(p, recycleDirPerms); err != nil {
			return err
		}
	}
	return nil
}

// recycleEntry moves src to target as the user. If target is a folder and src
// too, their content is merged. Any other item at target is a collision, and
// src gets the suffix .<inode>.
func (fs *cephmountfs) recycleEntry(ctx context.Context, src string, info os.FileInfo, target string) error {
	existing, err := fs.rootFS.Lstat(target)
	switch {
	case os.IsNotExist(err):
		return fs.renameIntoRecycle(ctx, src, target)
	case err != nil:
		return err
	case info.IsDir() && existing.IsDir():
		return fs.mergeIntoRecycle(ctx, src, target)
	}

	// rename(2) replaces an existing file without an error, so check the new name too.
	target += "." + strconv.FormatUint(info.Sys().(*syscall.Stat_t).Ino, 10)
	if _, err := fs.rootFS.Lstat(target); err == nil {
		return errtypes.AlreadyExists("cephmount: " + target + " is already in the recycle bin")
	} else if !os.IsNotExist(err) {
		return err
	}
	return fs.renameIntoRecycle(ctx, src, target)
}

// renameIntoRecycle renames src to target as the user. The bin folders are
// write-only for users, but os.Root opens each folder of a path, which needs read
// access. O_PATH needs only search access, and RESOLVE_BENEATH keeps both paths
// inside the mount, as os.Root does.
func (fs *cephmountfs) renameIntoRecycle(ctx context.Context, src, target string) error {
	root, err := unix.Open(fs.chrootDir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: fs.chrootDir, Err: err}
	}
	defer unix.Close(root)

	return fs.executeOnUserThread(ctx, func() error {
		how := &unix.OpenHow{Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH}
		srcDir, err := unix.Openat2(root, filepath.Dir(src), how)
		if err != nil {
			return &os.PathError{Op: "openat2", Path: filepath.Dir(src), Err: err}
		}
		defer unix.Close(srcDir)
		targetDir, err := unix.Openat2(root, filepath.Dir(target), how)
		if err != nil {
			return &os.PathError{Op: "openat2", Path: filepath.Dir(target), Err: err}
		}
		defer unix.Close(targetDir)
		if err := unix.Renameat(srcDir, filepath.Base(src), targetDir, filepath.Base(target)); err != nil {
			return &os.LinkError{Op: "renameat", Old: src, New: target, Err: err}
		}
		return nil
	})
}

// mergeIntoRecycle walks only the folders that exist on both sides. Everything
// else is moved with one rename.
func (fs *cephmountfs) mergeIntoRecycle(ctx context.Context, src, target string) error {
	entries, err := fs.readDirectoryAsUser(ctx, src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := fs.recycleEntry(ctx, filepath.Join(src, e.Name()), e, filepath.Join(target, e.Name())); err != nil {
			return err
		}
	}
	return fs.removeAsUser(ctx, src)
}

// ListRecycle lists one folder of the recycle bin of the space at basePath. A key
// starts with the days it covers, 20261123 or 20261116-20261123, followed by the
// path in the day folders. Without a key, the root is listed for the days from
// and to, or for today. A file is one item per deletion, and a folder is merged
// over the days, so that the user sees one tree.
func (fs *cephmountfs) ListRecycle(ctx context.Context, basePath, key, relativePath string, from, to *typepb.Timestamp) ([]*provider.RecycleItem, error) {
	fs.logOperation(ctx, "ListRecycle", path.Join(basePath, key, relativePath))

	spaceRoot, bin, err := fs.spaceRecycle(ctx, basePath)
	if err != nil {
		return nil, err
	}

	var first, last, rel string
	// The storage provider gives the root as an empty key with relativePath "/".
	if key != "" {
		if first, last, rel, err = parseRecycleKey(path.Join(key, relativePath)); err != nil {
			return nil, err
		}
	} else {
		first = time.Now().UTC().Format(recycleKeyDay)
		last, rel = first, "."
		if from != nil && to != nil {
			first = time.Unix(int64(from.Seconds), 0).UTC().Format(recycleKeyDay)
			last = time.Unix(int64(to.Seconds), 0).UTC().Format(recycleKeyDay)
		}
	}
	scope := first
	if last != first {
		scope += "-" + last
	}

	var items []*provider.RecycleItem
	folders := map[string]*provider.RecycleItem{}
	for _, day := range fs.recycleDays(bin, first, last) {
		dir := filepath.Join(bin, day, rel)
		// Root reads the bin, thus a deleted symlink in the key must not lead it to another folder.
		if through, err := fs.throughSymlink(dir); err != nil {
			return nil, err
		} else if through {
			continue
		}
		entries, err := iofs.ReadDir(fs.rootFS.FS(), dir)
		if errors.Is(err, iofs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return nil, errors.Wrap(err, "cephmount: failed to read the recycle bin")
		}
		parent := filepath.Join(spaceRoot, fs.originalPath(filepath.Join(bin, day), rel))

		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue // purged since the read
			}
			deleted := uint64(info.Sys().(*syscall.Stat_t).Ctim.Sec)
			if !info.IsDir() {
				items = append(items, &provider.RecycleItem{
					Type:         provider.ResourceType_RESOURCE_TYPE_FILE,
					Key:          path.Join(strings.ReplaceAll(day, "/", ""), rel, e.Name()),
					Ref:          &provider.Reference{Path: fs.fromChroot(filepath.Join(parent, withoutSuffix(e.Name(), info)))},
					Size:         uint64(info.Size()),
					DeletionTime: &typepb.Timestamp{Seconds: deleted},
				})
				continue
			}

			// Later deletes of the same day can add to a folder, which its rctime reflects.
			itemPath := filepath.Join(dir, e.Name())
			if sec, ok := rctimeSeconds(fs.recursiveCtime(ctx, filepath.Join(fs.chrootDir, itemPath))); ok && sec > deleted {
				deleted = sec
			}
			size := fs.directorySize(itemPath)
			if f, ok := folders[e.Name()]; ok {
				f.Size += size
				if deleted > f.DeletionTime.Seconds {
					f.DeletionTime.Seconds = deleted
				}
				continue
			}
			f := &provider.RecycleItem{
				Type:         provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				Key:          path.Join(scope, rel, e.Name()),
				Ref:          &provider.Reference{Path: fs.fromChroot(filepath.Join(parent, withoutSuffix(e.Name(), info)))},
				Size:         size,
				DeletionTime: &typepb.Timestamp{Seconds: deleted},
				Opaque: &typepb.Opaque{Map: map[string]*typepb.OpaqueEntry{
					storage.RecycleItemBrowsable: {Decoder: "plain", Value: []byte("true")},
				}},
			}
			folders[e.Name()] = f
			items = append(items, f)
		}
	}
	return items, nil
}

// spaceRecycle returns the space root at basePath and its recycle bin. Reva uses
// the bin as root, thus the user must be able to write to the space.
func (fs *cephmountfs) spaceRecycle(ctx context.Context, basePath string) (spaceRoot, bin string, err error) {
	if _, ok := externalUser(ctx); ok {
		return "", "", errtypes.PermissionDenied("cephmount: external accounts cannot use the recycle bin")
	}

	spaceRoot = fs.toChroot(basePath)
	depth := 0
	if spaceRoot != "." {
		depth = len(strings.Split(spaceRoot, string(filepath.Separator)))
	}
	if depth != fs.conf.SpaceDepth || inRecycle(spaceRoot) {
		return "", "", errtypes.BadRequest("cephmount: " + basePath + " is not a space root")
	}

	writable, err := fs.executeAsUser(ctx, func() (any, error) {
		return canAccess(filepath.Join(fs.chrootDir, spaceRoot), unix.W_OK|unix.X_OK), nil
	})
	if err != nil {
		return "", "", err
	}
	if !writable.(bool) {
		return "", "", errtypes.PermissionDenied("cephmount: no write access to " + basePath)
	}
	return spaceRoot, filepath.Join(recycleDir, spaceRoot), nil
}

// parseRecycleKey splits a key into its first and last day, and the path in the day folders.
func parseRecycleKey(key string) (first, last, rel string, err error) {
	scope, rel, _ := strings.Cut(key, "/")
	first, last, found := strings.Cut(scope, "-")
	if !found {
		last = first
	}
	if rel == "" {
		rel = "."
	}
	_, errFirst := time.Parse(recycleKeyDay, first)
	_, errLast := time.Parse(recycleKeyDay, last)
	if errFirst != nil || errLast != nil || first > last || !filepath.IsLocal(rel) {
		return "", "", "", errtypes.BadRequest("cephmount: invalid recycle bin key " + key)
	}
	return first, last, rel, nil
}

// recycleDays returns the day folders of bin from first to last, newest first.
// Only the folders that exist are read, thus a long range costs nothing extra.
func (fs *cephmountfs) recycleDays(bin, first, last string) []string {
	sub, err := iofs.Sub(fs.rootFS.FS(), bin)
	if err != nil {
		return nil
	}
	// Glob ignores read errors, thus a space without a bin has no days.
	dirs, _ := iofs.Glob(sub, "[0-9][0-9][0-9][0-9]/[0-9][0-9]/[0-9][0-9]")
	var days []string
	for _, d := range slices.Backward(dirs) {
		if day := strings.ReplaceAll(d, "/", ""); day >= first && day <= last {
			days = append(days, d)
		}
	}
	return days
}

// originalPath removes the collision suffixes from rel, a path in the day folder.
func (fs *cephmountfs) originalPath(day, rel string) string {
	var parts []string
	p := day
	for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
		p = filepath.Join(p, part)
		if info, err := fs.rootFS.Lstat(p); err == nil {
			part = withoutSuffix(part, info)
		}
		parts = append(parts, part)
	}
	return filepath.Join(parts...)
}

// withoutSuffix removes the collision suffix. The suffix is the inode of the item
// itself, so that a name like report.2024 stays as it is.
func withoutSuffix(name string, info os.FileInfo) string {
	suffix := "." + strconv.FormatUint(info.Sys().(*syscall.Stat_t).Ino, 10)
	if base, ok := strings.CutSuffix(name, suffix); ok && base != "" {
		return base
	}
	return name
}

// RestoreRecycleItem moves the item of key back to its original path, and
// ignores restoreRef, as the eos driver does. A folder key restores the folder
// from each day of its range, newest first. Nothing is overwritten: an item that
// collides stays in the bin, and the call returns these items as
// storage.RecycleCollisions. With force, the item that is in the way is renamed
// to <name>.<inode> first, as EOS does.
func (fs *cephmountfs) RestoreRecycleItem(ctx context.Context, basePath, key, relativePath string, restoreRef *provider.Reference, force bool) error {
	fs.logOperation(ctx, "RestoreRecycleItem", path.Join(basePath, key, relativePath))

	spaceRoot, bin, err := fs.spaceRecycle(ctx, basePath)
	if err != nil {
		return err
	}
	first, last, rel, err := parseRecycleKey(path.Join(key, relativePath))
	if err != nil {
		return err
	}
	if rel == "." {
		return errtypes.BadRequest("cephmount: the key " + key + " does not name an item")
	}

	r := &recycleRestore{force: force, spaceRoot: spaceRoot}
	found := false
	for _, day := range fs.recycleDays(bin, first, last) {
		dayDir := filepath.Join(bin, day)
		r.day, r.dayDir = strings.ReplaceAll(day, "/", ""), dayDir
		src, info, err := fs.recycleItem(dayDir, rel)
		if err != nil {
			return err
		}
		if info == nil {
			continue
		}
		found = true

		target := filepath.Join(spaceRoot, fs.originalPath(dayDir, rel))
		if err := fs.restoreEntry(ctx, src, info, target, r); err != nil {
			if _, ok := err.(errtypes.IsPermissionDenied); ok {
				return err
			}
			return errors.Wrap(err, "cephmount: failed to restore "+fs.fromChroot(target))
		}
		fs.removeEmptyRecycleDirs(src, dayDir)
	}

	if !found {
		return errtypes.NotFound("cephmount: no item in the recycle bin for " + path.Join(key, relativePath))
	}
	if len(r.collided) > 0 {
		return r.collided
	}
	return nil
}

// recycleRestore keeps what one restore did. An older copy of an item that the
// same restore put back stays in the bin, and is not a collision.
type recycleRestore struct {
	force     bool
	spaceRoot string
	// The day folder that is restored, for the key of a collision.
	day, dayDir string
	restored    []string
	collided    storage.RecycleCollisions
}

// restoreEntry is the reverse of recycleEntry. Root does the move after a check
// that the user can write to the target folder, so that any writer of the space
// can restore an item, whoever deleted or owns it. Only root can list the bin,
// thus root reads it.
func (fs *cephmountfs) restoreEntry(ctx context.Context, src string, info os.FileInfo, target string, r *recycleRestore) error {
	existing, err := fs.rootFS.Lstat(target)
	switch {
	case os.IsNotExist(err):
		if err := fs.createDirectoryAsUser(ctx, filepath.Dir(target), os.FileMode(fs.conf.DirPerms)); err != nil {
			return err
		}
		if err := fs.writableByUser(ctx, filepath.Dir(target)); err != nil {
			return err
		}
		// One move would also restore the older copies of a path, with their suffix.
		if info.IsDir() && fs.holdsCopies(src) {
			if err := fs.createDirectoryAsUser(ctx, target, os.FileMode(fs.conf.DirPerms)); err != nil {
				return err
			}
			r.restored = append(r.restored, target)
			return fs.mergeFromRecycle(ctx, src, target, r)
		}
		// Before the move, so that a failure cannot leave old access on a restored item.
		if err := fs.clearGrants(ctx, src); err != nil {
			return err
		}
		if err := fs.rootFS.Rename(src, target); err != nil {
			return err
		}
		r.restored = append(r.restored, target)
		return fs.inheritACL(ctx, target, info.IsDir())
	case err != nil:
		return err
	case info.IsDir() && existing.IsDir():
		return fs.mergeFromRecycle(ctx, src, target, r)
	}

	if slices.ContainsFunc(r.restored, func(p string) bool {
		return target == p || strings.HasPrefix(target, p+string(filepath.Separator))
	}) {
		return nil
	}

	if r.force {
		if err := fs.writableByUser(ctx, filepath.Dir(target)); err != nil {
			return err
		}
		// rename(2) replaces an existing file without an error, so check the new name too.
		aside := target + "." + strconv.FormatUint(existing.Sys().(*syscall.Stat_t).Ino, 10)
		if _, err := fs.rootFS.Lstat(aside); err == nil {
			return errtypes.AlreadyExists("cephmount: " + fs.fromChroot(aside) + " already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := fs.rootFS.Rename(target, aside); err != nil {
			return err
		}
		return fs.restoreEntry(ctx, src, info, target, r)
	}

	// The older copies of a path collide with the same item, thus it is reported once.
	rel, err := filepath.Rel(r.spaceRoot, target)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(r.collided, func(c storage.RecycleCollision) bool { return c.Path == rel }) {
		inDay, err := filepath.Rel(r.dayDir, src)
		if err != nil {
			return err
		}
		r.collided = append(r.collided, storage.RecycleCollision{Key: path.Join(r.day, inDay), Path: rel})
	}
	return nil
}

// holdsCopies reports whether an item below the folder dir has the collision
// suffix. If the bin cannot be read, it reports true: a restore item by item is
// correct in all cases, only slower.
func (fs *cephmountfs) holdsCopies(dir string) bool {
	found := false
	err := iofs.WalkDir(fs.rootFS.FS(), dir, func(p string, e iofs.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		if withoutSuffix(e.Name(), info) != e.Name() {
			found = true
			return iofs.SkipAll
		}
		return nil
	})
	return found || err != nil
}

// writableByUser returns PermissionDenied if the user cannot create and remove
// items in dir. Root moves the items, thus the kernel does not check the user.
func (fs *cephmountfs) writableByUser(ctx context.Context, dir string) error {
	writable, err := fs.executeAsUser(ctx, func() (any, error) {
		return canAccess(filepath.Join(fs.chrootDir, dir), unix.W_OK|unix.X_OK), nil
	})
	if err != nil {
		return err
	}
	if !writable.(bool) {
		return errtypes.PermissionDenied("cephmount: no write access to " + fs.fromChroot(dir))
	}
	return nil
}

// mergeFromRecycle restores the content of the folder src into the folder target.
func (fs *cephmountfs) mergeFromRecycle(ctx context.Context, src, target string, r *recycleRestore) error {
	entries, err := iofs.ReadDir(fs.rootFS.FS(), src)
	if err != nil {
		return err
	}
	infos := make([]os.FileInfo, 0, len(entries))
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			infos = append(infos, info)
		}
	}
	// One folder can hold several deletions of the same path: the newest goes first.
	slices.SortFunc(infos, func(a, b os.FileInfo) int {
		ca, cb := a.Sys().(*syscall.Stat_t).Ctim, b.Sys().(*syscall.Stat_t).Ctim
		return cmp.Or(cmp.Compare(cb.Sec, ca.Sec), cmp.Compare(cb.Nsec, ca.Nsec))
	})

	for _, info := range infos {
		if err := fs.restoreEntry(ctx, filepath.Join(src, info.Name()), info, filepath.Join(target, withoutSuffix(info.Name(), info)), r); err != nil {
			return err
		}
	}
	// Fails if a collision stays in it.
	_ = fs.rootFS.Remove(src)
	return nil
}

// clearGrants removes the ACL entries, the external account grants and the lock
// of path and of all items below it. Old grants must not come back with a
// restore: the grants of the folder above can have changed since the delete.
func (fs *cephmountfs) clearGrants(ctx context.Context, path string) error {
	fullPath := filepath.Join(fs.chrootDir, path)
	// -P, so that root never changes the ACL of the target of a symlink.
	if err := fs.runSetfacl(ctx, []string{"-R", "-P", "-b", fullPath}, fullPath); err != nil {
		return err
	}
	return filepath.WalkDir(fullPath, func(p string, _ iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		names, err := xattr.LList(p)
		if err != nil {
			return err
		}
		for _, name := range names {
			if name == xattrLock || strings.HasPrefix(name, xattrExtShare) {
				if err := xattr.LRemove(p, name); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// inheritACL applies the named entries of the default ACL of the folder above
// path, as AddGrant applies a grant: to path and all items below it, and as
// default entries to folders. The mode bits stay the ones from before the delete.
func (fs *cephmountfs) inheritACL(ctx context.Context, path string, isDir bool) error {
	fullPath := filepath.Join(fs.chrootDir, path)
	parent := filepath.Dir(fullPath)
	output, err := exec.CommandContext(ctx, "getfacl", "--default", "--omit-header", "--numeric", "--no-effective", parent).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "cephmount: getfacl failed: %s", strings.TrimSpace(string(output)))
	}

	var entries []string
	for line := range strings.Lines(string(output)) {
		line = strings.TrimSpace(line)
		kind, rest, _ := strings.Cut(line, ":")
		if id, _, _ := strings.Cut(rest, ":"); (kind == "user" || kind == "group") && id != "" {
			entries = append(entries, line)
		}
	}
	if len(entries) == 0 {
		return nil
	}

	args := []string{"-R", "-P", "-m", strings.Join(entries, ",")}
	if isDir {
		args = append(args, "-m", "d:"+strings.Join(entries, ",d:"))
	}
	return fs.runSetfacl(ctx, append(args, fullPath), fullPath)
}

// recycleItem returns the item rel in a day folder, with no info if it is not
// there. The item can be a symlink, but no folder above it can: root changes the
// item, and must not be led to another folder.
func (fs *cephmountfs) recycleItem(dayDir, rel string) (string, os.FileInfo, error) {
	src := filepath.Join(dayDir, rel)
	if through, err := fs.throughSymlink(filepath.Dir(src)); err != nil || through {
		return src, nil, err
	}
	info, err := fs.rootFS.Lstat(src)
	if errors.Is(err, iofs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return src, nil, nil
	}
	if err != nil {
		return src, nil, errors.Wrap(err, "cephmount: failed to read the recycle bin")
	}
	return src, info, nil
}

// removeEmptyRecycleDirs removes the bin folders above src that are now empty,
// up to the day folder. Otherwise the listing shows them.
func (fs *cephmountfs) removeEmptyRecycleDirs(src, dayDir string) {
	for d := filepath.Dir(src); d != dayDir; d = filepath.Dir(d) {
		if fs.rootFS.Remove(d) != nil {
			return
		}
	}
}
