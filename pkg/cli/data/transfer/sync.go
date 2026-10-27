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
	"cmp"
	"context"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// Direction selects the side of a sync that the other side must match.
type Direction int

const (
	Push Direction = iota
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
	Direction Direction
	// Delete removes what the destination has and the source does not.
	Delete        bool
	IncludeHidden bool
	// Exclude holds glob patterns, see Excluded.
	Exclude []string
}

// Excluded reports whether rel, a slash path relative to the sync root, matches
// an exclude pattern. A pattern with a slash is matched against the whole path,
// and one without against each component, so "*.tmp" matches in any directory
// and an excluded directory excludes everything below it.
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

// skip hides an entry on both sides, otherwise --delete would remove an
// excluded entry from the destination.
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

type syncEntry struct {
	rel      string
	isDir    bool
	size     int64
	modified time.Time
}

// Sync mirrors one side onto the other. It is one-way on purpose: two-way sync
// needs per-file state to tell "changed here" from "deleted there", or it
// deletes data that it should upload.
func (e *Engine) Sync(ctx context.Context, localRoot, remoteRoot string, opts SyncOptions) (*SyncStats, error) {
	start := time.Now()
	stats := &SyncStats{}

	// A pull creates its target. A push does not create its source: an empty
	// source would mirror an empty tree over the server.
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
	// A flat source has no subdirectory that creates the root as a side effect.
	if !e.opts.DryRun {
		if err := e.c.Mkdir(ctx, remoteRoot, true); err != nil {
			return err
		}
	}

	for _, rel := range sortedDirs(local) {
		if _, exists := remote[rel]; exists {
			continue
		}
		stats.Dirs++
		if e.opts.DryRun {
			e.emit(Event{Path: path.Join(remoteRoot, rel), Done: true})
			continue
		}
		if err := e.c.Mkdir(ctx, path.Join(remoteRoot, rel), true); err != nil {
			return err
		}
	}

	files := filesNeedingTransfer(local, remote, stats)

	var mu sync.Mutex
	err := e.eachParallel(ctx, len(files), func(ctx context.Context, i int) error {
		rel := files[i].rel
		localPath := filepath.Join(localRoot, filepath.FromSlash(rel))
		remotePath := path.Join(remoteRoot, rel)

		n, err := e.withOverwrite().UploadFile(ctx, localPath, remotePath)
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

		n, err := e.withOverwrite().DownloadFile(ctx, remotePath, localPath)
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

type transferItem struct {
	rel     string
	existed bool
}

// filesNeedingTransfer returns the files that are missing on the destination,
// of another size, or newer on the source. Like rsync, it does not read the
// content.
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
			// Replacing a directory would delete a subtree.
			stats.Skipped++
		case src.size != dst.size || src.modified.After(dst.modified.Add(time.Second)):
			out = append(out, transferItem{rel: rel, existed: true})
		default:
			stats.Skipped++
		}
	}
	slices.SortFunc(out, func(a, b transferItem) int { return strings.Compare(a.rel, b.rel) })
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
			// The parent was removed before, with its children.
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
		// Same as in walkLocal: skip links.
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

// scanRemote treats a missing root as an empty tree, so that a first push works.
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

// withOverwrite returns an engine that replaces existing files, because a sync
// that skips them would report success with different sides.
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
	slices.SortFunc(dirs, byDepth)
	return dirs
}

func deepestFirst(entries map[string]syncEntry) []string {
	all := slices.SortedFunc(maps.Keys(entries), byDepth)
	slices.Reverse(all)
	return all
}

// byDepth sorts slash paths shallowest first, then by name.
func byDepth(a, b string) int {
	return cmp.Or(cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")), strings.Compare(a, b))
}
