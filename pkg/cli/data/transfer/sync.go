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
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// Direction selects which side of a sync is authoritative.
type Direction int

const (
	// Push makes the local tree authoritative: the server is made to match it.
	Push Direction = iota
	// Pull makes the server authoritative: the local tree is made to match it.
	Pull
)

func (d Direction) String() string {
	if d == Pull {
		return "pull"
	}
	return "push"
}

// SyncOptions configures a mirror.
type SyncOptions struct {
	// Direction selects the authoritative side.
	Direction Direction
	// Delete removes entries from the destination that the source does not
	// have. Without it, sync only adds and updates.
	Delete bool
	// IncludeHidden syncs entries whose name begins with a dot.
	IncludeHidden bool
	// Exclude holds glob patterns for entries to leave out of the mirror
	// altogether. See Excluded for how a pattern is matched.
	Exclude []string
}

// Excluded reports whether rel, a slash-separated path relative to the sync
// root, matches any of the exclude patterns.
//
// A pattern containing a slash is matched against the whole relative path, and
// one without against each component, so "*.tmp" excludes a temporary file
// wherever it appears while "build/*" excludes only what is directly under a
// top-level build directory. Matching a component also excludes everything
// below it: a pattern that names a directory means the directory.
func (o SyncOptions) Excluded(rel string) bool {
	for _, pattern := range o.Exclude {
		if pattern == "" {
			continue
		}
		if strings.Contains(pattern, "/") {
			if ok, err := path.Match(pattern, rel); err == nil && ok {
				return true
			}
			continue
		}
		for part := range strings.SplitSeq(rel, "/") {
			if ok, err := path.Match(pattern, part); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// skip reports whether an entry is not part of the mirror at all, in either
// direction. An excluded entry is invisible rather than merely not copied: were
// it visible on the destination side, --delete would remove it for not being in
// the source, which is the opposite of what excluding something means.
func (o SyncOptions) skip(rel string) bool {
	return (!o.IncludeHidden && hasHiddenComponent(rel)) || o.Excluded(rel)
}

// SyncStats reports what a mirror did.
type SyncStats struct {
	Created  int           `json:"created"`
	Updated  int           `json:"updated"`
	Deleted  int           `json:"deleted"`
	Skipped  int           `json:"skipped"`
	Dirs     int           `json:"dirs"`
	Bytes    int64         `json:"bytes"`
	Duration time.Duration `json:"duration_ns"`
}

// syncEntry is one file on one side of the comparison.
type syncEntry struct {
	rel      string
	isDir    bool
	size     int64
	modified time.Time
}

// Sync mirrors one side onto the other.
//
// This is deliberately a one-way mirror, not two-way synchronisation. Genuine
// bidirectional sync needs persistent per-file state to tell "changed here"
// from "deleted there", and without it the two cases are indistinguishable ,
// which is how a sync tool deletes data it should have uploaded. That state is
// the desktop client's job; this command answers "make the far side look like
// this one", which is what a script wants.
func (e *Engine) Sync(ctx context.Context, localRoot, remoteRoot string, opts SyncOptions) (*SyncStats, error) {
	start := time.Now()
	stats := &SyncStats{}

	// Pulling into a directory that is not there yet is the ordinary way to
	// take a first copy of something, so create it rather than refusing. Push
	// keeps the stricter reading: a missing source is a mistake, and creating
	// it would mirror an empty tree over whatever is on the far side.
	if opts.Direction == Pull {
		if _, err := os.Stat(localRoot); os.IsNotExist(err) {
			if err := os.MkdirAll(localRoot, 0o755); err != nil {
				return nil, cberr.Wrap(cberr.KindOther, "create", localRoot, err)
			}
		}
	}

	localEntries, err := e.scanLocal(localRoot, opts)
	if err != nil {
		return nil, err
	}
	remoteEntries, err := e.scanRemote(ctx, remoteRoot, opts)
	if err != nil {
		return nil, err
	}

	if opts.Direction == Push {
		err = e.syncPush(ctx, localRoot, remoteRoot, localEntries, remoteEntries, opts, stats)
	} else {
		err = e.syncPull(ctx, localRoot, remoteRoot, localEntries, remoteEntries, opts, stats)
	}
	stats.Duration = time.Since(start)
	return stats, err
}

func (e *Engine) syncPush(ctx context.Context, localRoot, remoteRoot string, local, remote map[string]syncEntry, opts SyncOptions, stats *SyncStats) error {
	// The destination root may not exist yet, and a flat source tree has no
	// subdirectories to create it as a side effect. Without this, uploading
	// into a fresh destination fails with a conflict on the first file.
	if !e.opts.DryRun {
		if err := e.c.Mkdir(ctx, remoteRoot, true); err != nil {
			return err
		}
	}

	// Then the rest, shallowest to deepest, so every file has a parent.
	for _, rel := range sortedDirs(local) {
		if _, exists := remote[rel]; exists {
			continue
		}
		stats.Dirs++
		if e.opts.DryRun {
			e.emit(Event{Path: path.Join(remoteRoot, rel), Done: true})
			continue
		}
		if err := e.c.Mkdir(ctx, path.Join(remoteRoot, filepath.ToSlash(rel)), true); err != nil {
			return err
		}
	}

	files := filesNeedingTransfer(local, remote, stats)

	var mu sync.Mutex
	err := e.eachParallel(ctx, len(files), func(ctx context.Context, i int) error {
		rel := files[i].rel
		localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
		remotePath := path.Join(remoteRoot, rel)

		// Sync is an explicit "make it match", so an existing destination is
		// replaced rather than skipped: unlike cp, where skipping is the safe
		// default.
		engine := e.withOverwrite()
		n, err := engine.UploadFile(ctx, localPath, remotePath)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			return err
		}
		stats.Bytes += n
		if files[i].existed {
			stats.Updated++
		} else {
			stats.Created++
		}
		return nil
	})
	if err != nil {
		return err
	}

	if !opts.Delete {
		return nil
	}
	return e.deleteRemoteExtras(ctx, remoteRoot, local, remote, stats)
}

func (e *Engine) syncPull(ctx context.Context, localRoot, remoteRoot string, local, remote map[string]syncEntry, opts SyncOptions, stats *SyncStats) error {
	for _, rel := range sortedDirs(remote) {
		if _, exists := local[rel]; exists {
			continue
		}
		stats.Dirs++
		if e.opts.DryRun {
			continue
		}
		if err := os.MkdirAll(filepath.Join(localRoot, filepath.FromSlash(rel)), 0o755); err != nil {
			return cberr.Wrap(cberr.KindOther, "create", localRoot, err)
		}
	}

	files := filesNeedingTransfer(remote, local, stats)

	var mu sync.Mutex
	err := e.eachParallel(ctx, len(files), func(ctx context.Context, i int) error {
		rel := files[i].rel
		localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
		remotePath := path.Join(remoteRoot, rel)

		engine := e.withOverwrite()
		n, err := engine.DownloadFile(ctx, remotePath, localPath)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			return err
		}
		stats.Bytes += n
		if files[i].existed {
			stats.Updated++
		} else {
			stats.Created++
		}
		return nil
	})
	if err != nil {
		return err
	}

	if !opts.Delete {
		return nil
	}
	return e.deleteLocalExtras(localRoot, remote, local, stats)
}

// transferItem is a file the comparison decided to move.
type transferItem struct {
	rel     string
	existed bool
}

// filesNeedingTransfer compares the two sides and returns what must move.
//
// A file is transferred when it is missing on the destination, or when its size
// differs, or when the source is newer. Comparing content would be correct but
// would mean reading every byte on both sides, which defeats the purpose; size
// plus mtime is the same trade rsync makes.
func filesNeedingTransfer(source, dest map[string]syncEntry, stats *SyncStats) []transferItem {
	var out []transferItem
	for rel, src := range source {
		if src.isDir {
			continue
		}
		dst, exists := dest[rel]
		switch {
		case !exists:
			out = append(out, transferItem{rel: rel})
		case dst.isDir:
			// The name is a directory on the other side. Replacing it would
			// mean deleting a subtree, which a sync that only adds and updates
			// must not do silently.
			stats.Skipped++
		case src.size != dst.size || src.modified.After(dst.modified.Add(time.Second)):
			out = append(out, transferItem{rel: rel, existed: true})
		default:
			stats.Skipped++
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	return out
}

func (e *Engine) deleteRemoteExtras(ctx context.Context, remoteRoot string, local, remote map[string]syncEntry, stats *SyncStats) error {
	// Deepest first, so a directory is empty by the time it is removed.
	for _, rel := range deepestFirst(remote) {
		if _, kept := local[rel]; kept {
			continue
		}
		stats.Deleted++
		if e.opts.DryRun {
			continue
		}
		if err := e.c.Remove(ctx, path.Join(remoteRoot, rel)); err != nil {
			// A parent directory removed a moment ago takes its children with
			// it, so a missing child here is expected, not a failure.
			if backend.IsNotFound(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func (e *Engine) deleteLocalExtras(localRoot string, remote, local map[string]syncEntry, stats *SyncStats) error {
	for _, rel := range deepestFirst(local) {
		if _, kept := remote[rel]; kept {
			continue
		}
		stats.Deleted++
		if e.opts.DryRun {
			continue
		}
		target := filepath.Join(localRoot, filepath.FromSlash(rel))
		if err := os.RemoveAll(target); err != nil {
			return cberr.Wrap(cberr.KindOther, "remove", target, err)
		}
	}
	return nil
}

// scanLocal indexes the local tree by path relative to the root.
func (e *Engine) scanLocal(root string, opts SyncOptions) (map[string]syncEntry, error) {
	out := map[string]syncEntry{}
	info, err := os.Stat(root)
	if err != nil {
		return nil, cberr.Wrap(cberr.KindNotFound, "read", root, err)
	}
	if !info.IsDir() {
		return nil, cberr.Usagef("%s is not a directory: sync mirrors directories", root)
	}

	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if opts.skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Symlinks are not followed, for the same reason as in a plain upload:
		// copying the target under the link's name changes the shape of the
		// tree the user asked to mirror.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		fi, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		out[rel] = syncEntry{rel: rel, isDir: d.IsDir(), size: fi.Size(), modified: fi.ModTime()}
		return nil
	})
	if err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "read", root, err)
	}
	return out, nil
}

// scanRemote indexes the the server tree by path relative to the root. A root that
// does not exist yet is an empty tree, so a first push works without a
// preparatory mkdir.
func (e *Engine) scanRemote(ctx context.Context, root string, opts SyncOptions) (map[string]syncEntry, error) {
	out := map[string]syncEntry{}

	info, err := e.c.Stat(ctx, root)
	if err != nil {
		if backend.IsNotFound(err) {
			return out, nil
		}
		return nil, err
	}
	if !info.IsDir {
		return nil, cberr.Usagef("%s is not a directory: sync mirrors directories", root)
	}

	err = e.c.Walk(ctx, root, func(item ResourceInfo) error {
		if item.Path == root {
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
		if rel == "" {
			return nil
		}
		if opts.skip(rel) {
			return nil
		}
		out[rel] = syncEntry{rel: rel, isDir: item.IsDir, size: item.Size, modified: item.Modified}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// withOverwrite returns an engine that replaces existing destinations. Sync
// means "make it match", so skipping would leave the sides different and report
// success.
func (e *Engine) withOverwrite() *Engine {
	if e.opts.Overwrite {
		return e
	}
	opts := e.opts
	opts.Overwrite = true
	return &Engine{c: e.c, opts: opts}
}

func hasHiddenComponent(rel string) bool {
	for part := range strings.SplitSeq(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func sortedDirs(entries map[string]syncEntry) []string {
	var dirs []string
	for rel, e := range entries {
		if e.isDir {
			dirs = append(dirs, rel)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		di, dj := strings.Count(dirs[i], "/"), strings.Count(dirs[j], "/")
		if di != dj {
			return di < dj
		}
		return dirs[i] < dirs[j]
	})
	return dirs
}

func deepestFirst(entries map[string]syncEntry) []string {
	all := make([]string, 0, len(entries))
	for rel := range entries {
		all = append(all, rel)
	}
	sort.Slice(all, func(i, j int) bool {
		di, dj := strings.Count(all[i], "/"), strings.Count(all[j], "/")
		if di != dj {
			return di > dj
		}
		return all[i] > all[j]
	})
	return all
}
