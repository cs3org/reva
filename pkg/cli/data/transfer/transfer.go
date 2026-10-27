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

// Package transfer moves data between the local filesystem and the server, on
// the metadata calls of a backend and the data plane.
//
// It exists to make three things routine rather than exceptional: an
// interrupted transfer resumes instead of restarting, many small files go in
// parallel instead of one round trip at a time, and what arrives is verified
// against what was sent.
package transfer

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"hash/adler32"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// defaultChunkSize is the upload chunk used when the server advertises no
// limit. Large enough that per-chunk overhead is negligible, small enough that
// an interrupted transfer loses little.
const defaultChunkSize = 8 << 20

// defaultPutThreshold is the size below which a plain PUT beats TUS: setting up
// a resumable upload costs two extra round trips, which dominates for a small
// file.
const defaultPutThreshold = 4 << 20

// Event reports transfer progress.
type Event struct {
	// Path is the resource being transferred, as the user named it.
	Path string
	// Transferred is how many bytes have moved so far.
	Transferred int64
	// Total is the size of the resource, or -1 when unknown.
	Total int64
	// Done reports whether this resource has finished.
	Done bool
	// Err is set when this resource failed.
	Err error
}

// ProgressFunc receives progress events. It may be called from several
// goroutines at once, so implementations must be safe for concurrent use.
type ProgressFunc func(Event)

// Options configures an Engine.
type Options struct {
	// Jobs is the number of resources transferred concurrently.
	Jobs int
	// ChunkSize is the preferred upload chunk, clamped to what the server
	// advertises.
	ChunkSize int64
	// PutThreshold is the size at or above which an upload becomes resumable.
	// Below it a plain PUT is faster, because setting up a TUS upload costs two
	// extra round trips.
	PutThreshold int64
	// DryRun reports what would happen without transferring anything.
	DryRun bool
	// Overwrite allows replacing an existing destination. Without it, an
	// existing destination is skipped rather than clobbered.
	Overwrite bool
	// Verify computes a checksum and asks the server to verify it.
	Verify bool
	// Archive allows a recursive download to be served by the archiver as a
	// single stream, instead of one request per file.
	Archive bool
	// StateDir holds resumable-upload state between invocations.
	StateDir string
	// Progress receives events.
	Progress ProgressFunc
}

func (o *Options) applyDefaults() {
	if o.Jobs <= 0 {
		o.Jobs = min(8, max(2, runtime.NumCPU()))
	}
	if o.ChunkSize <= 0 {
		o.ChunkSize = defaultChunkSize
	}
	if o.PutThreshold <= 0 {
		o.PutThreshold = defaultPutThreshold
	}
	if o.StateDir == "" {
		o.StateDir = DefaultStateDir("reva")
	}
}

// DefaultStateDir returns where resumable-upload state is kept, for the
// program with the config dir name.
func DefaultStateDir(name string) string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, name, "uploads")
	}
	return filepath.Join(os.TempDir(), name+"-uploads")
}

// Stats summarises a transfer.
type Stats struct {
	Files    int           `json:"files"`
	Dirs     int           `json:"dirs"`
	Bytes    int64         `json:"bytes"`
	Skipped  int           `json:"skipped"`
	Duration time.Duration `json:"duration_ns"`
}

// Engine performs transfers.
type Engine struct {
	c    remote
	opts Options
}

// New returns an Engine on the metadata calls f and the data plane d of the
// same server.
func New(f backend.Files, d backend.DataPlane, opts Options) *Engine {
	opts.applyDefaults()
	return &Engine{c: remote{f: f, d: d}, opts: opts}
}

// Options returns the effective options, after defaults.
func (e *Engine) Options() Options { return e.opts }

func (e *Engine) emit(ev Event) {
	if e.opts.Progress != nil {
		e.opts.Progress(ev)
	}
}

// -- upload -------------------------------------------------------------------

// UploadFile sends one local file to a the server path.
func (e *Engine) UploadFile(ctx context.Context, localPath, remotePath string) (int64, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return 0, cberr.Wrap(cberr.KindNotFound, "read", localPath, err)
	}
	if info.IsDir() {
		return 0, cberr.Usagef("%s is a directory: pass -r to copy it recursively", localPath)
	}

	if !e.opts.Overwrite {
		if _, statErr := e.c.Stat(ctx, remotePath); statErr == nil {
			e.emit(Event{Path: remotePath, Total: info.Size(), Done: true})
			return 0, errSkipped
		} else if !backend.IsNotFound(statErr) {
			return 0, statErr
		}
	}

	if e.opts.DryRun {
		e.emit(Event{Path: remotePath, Transferred: info.Size(), Total: info.Size(), Done: true})
		return info.Size(), nil
	}

	checksum := ""
	if e.opts.Verify {
		checksum, err = e.fileChecksum(ctx, localPath)
		if err != nil {
			return 0, err
		}
	}

	caps := e.c.caps(ctx)
	if !caps.TUS || info.Size() < e.opts.PutThreshold {
		if err := e.simpleUpload(ctx, localPath, remotePath, info.Size(), checksum); err != nil {
			return 0, err
		}
		e.emit(Event{Path: remotePath, Transferred: info.Size(), Total: info.Size(), Done: true})
		return info.Size(), nil
	}

	return e.resumableUpload(ctx, localPath, remotePath, info, checksum)
}

// errSkipped signals that a resource was intentionally not transferred. It is
// handled inside the engine and never reaches a caller.
var errSkipped = errors.New("skipped")

// simpleUpload lets the data plane compute the checksum of the type that
// the server prefers, when checksum says to verify.
func (e *Engine) simpleUpload(ctx context.Context, localPath, remotePath string, size int64, checksum string) error {
	body := func() (io.ReadCloser, error) { return os.Open(localPath) }
	xs := ""
	if checksum != "" {
		xs = "negotiate"
	}
	return e.c.Upload(ctx, remotePath, body, size, xs)
}

// resumableUpload transfers a large file with TUS, continuing a previous
// attempt when one is recorded. The state is saved after each part, so
// that an interrupted transfer continues on the next run.
func (e *Engine) resumableUpload(ctx context.Context, localPath, remotePath string, info os.FileInfo, checksum string) (int64, error) {
	open := func() (io.ReadCloser, error) { return os.Open(localPath) }
	var last backend.Upload
	progress := func(up backend.Upload) {
		last = up
		e.saveState(localPath, remotePath, &up, up.Offset)
		e.emit(Event{Path: remotePath, Transferred: up.Offset, Total: info.Size()})
	}
	start := int64(0)
	var err error
	if up, ok := e.loadState(localPath, remotePath, info); ok {
		start = up.Offset
		// The offset of the server is the only one to trust: the recorded
		// progress can be ahead of what it committed.
		err = e.c.d.ResumeUpload(ctx, *up, open, progress)
		if backend.IsNotFound(err) {
			// The upload that the server knew is gone: start a new one rather
			// than fail with a state file that the user does not know of.
			e.clearState(localPath, remotePath)
			start = 0
			err = e.c.d.UploadResumable(ctx, ref(remotePath), open, info.Size(), checksum, progress)
		}
	} else {
		err = e.c.d.UploadResumable(ctx, ref(remotePath), open, info.Size(), checksum, progress)
	}
	if errors.Is(err, backend.ErrNotAvailable) {
		e.clearState(localPath, remotePath)
		if err := e.simpleUpload(ctx, localPath, remotePath, info.Size(), checksum); err != nil {
			return 0, err
		}
		e.emit(Event{Path: remotePath, Transferred: info.Size(), Total: info.Size(), Done: true})
		return info.Size(), nil
	}
	if err != nil {
		return max(last.Offset-start, 0), err
	}
	e.clearState(localPath, remotePath)
	e.emit(Event{Path: remotePath, Transferred: info.Size(), Total: info.Size(), Done: true})
	return info.Size() - start, nil
}

// UploadTree copies a local directory tree to the server.
func (e *Engine) UploadTree(ctx context.Context, localRoot, remoteRoot string) (*Stats, error) {
	start := time.Now()
	stats := &Stats{}

	info, err := os.Stat(localRoot)
	if err != nil {
		return nil, cberr.Wrap(cberr.KindNotFound, "read", localRoot, err)
	}
	if !info.IsDir() {
		n, err := e.UploadFile(ctx, localRoot, remoteRoot)
		if errors.Is(err, errSkipped) {
			stats.Skipped++
			stats.Duration = time.Since(start)
			return stats, nil
		}
		if err != nil {
			return nil, err
		}
		stats.Files, stats.Bytes, stats.Duration = 1, n, time.Since(start)
		return stats, nil
	}

	dirs, files, err := walkLocal(localRoot)
	if err != nil {
		return nil, err
	}

	// Directories first, shallowest to deepest, so every file has a parent by
	// the time it is uploaded.
	for _, rel := range dirs {
		remote := path.Join(remoteRoot, filepath.ToSlash(rel))
		if e.opts.DryRun {
			stats.Dirs++
			continue
		}
		if err := e.c.Mkdir(ctx, remote, true); err != nil {
			return nil, err
		}
		stats.Dirs++
	}
	if e.opts.DryRun {
		// A dry run still needs the root itself counted.
		stats.Dirs++
	} else if err := e.c.Mkdir(ctx, remoteRoot, true); err != nil {
		return nil, err
	}

	var mu sync.Mutex
	err = e.eachParallel(ctx, len(files), func(ctx context.Context, i int) error {
		rel := files[i]
		local := filepath.Join(localRoot, rel)
		remote := path.Join(remoteRoot, filepath.ToSlash(rel))

		n, err := e.UploadFile(ctx, local, remote)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case errors.Is(err, errSkipped):
			stats.Skipped++
			return nil
		case err != nil:
			return err
		default:
			stats.Files++
			stats.Bytes += n
			return nil
		}
	})
	stats.Duration = time.Since(start)
	if err != nil {
		return stats, err
	}
	return stats, nil
}

// -- download -----------------------------------------------------------------

// DownloadFile fetches one the server file to a local path, resuming a partial
// transfer when one is present.
func (e *Engine) DownloadFile(ctx context.Context, remotePath, localPath string) (int64, error) {
	info, err := e.c.Stat(ctx, remotePath)
	if err != nil {
		return 0, err
	}
	if info.IsDir {
		return 0, cberr.Usagef("%s is a directory: pass -r to copy it recursively", remotePath)
	}

	if !e.opts.Overwrite {
		if _, err := os.Stat(localPath); err == nil {
			e.emit(Event{Path: localPath, Total: info.Size, Done: true})
			return 0, errSkipped
		}
	}
	if e.opts.DryRun {
		e.emit(Event{Path: localPath, Transferred: info.Size, Total: info.Size, Done: true})
		return info.Size, nil
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
		return 0, cberr.Wrap(cberr.KindOther, "create", filepath.Dir(localPath), err)
	}

	// Download into a sibling .part file. A consumer watching the destination
	// never sees a truncated file, and an interrupted transfer leaves something
	// to resume from.
	partPath := localPath + ".part"
	offset := int64(0)
	if st, err := os.Stat(partPath); err == nil && st.Size() < info.Size {
		offset = st.Size()
	}

	body, _, err := e.c.Download(ctx, remotePath, offset)
	if err != nil {
		return 0, err
	}
	defer body.Close()

	flags := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(partPath, flags, 0o644)
	if err != nil {
		return 0, cberr.Wrap(cberr.KindOther, "create", partPath, err)
	}

	written, err := e.copyWithProgress(ctx, f, body, localPath, offset, info.Size)
	closeErr := f.Close()
	if err != nil {
		return written, err
	}
	if closeErr != nil {
		return written, cberr.Wrap(cberr.KindOther, "write", partPath, closeErr)
	}

	if err := os.Rename(partPath, localPath); err != nil {
		return written, cberr.Wrap(cberr.KindOther, "write", localPath, err)
	}

	if e.opts.Verify && len(info.Checksums) > 0 {
		if err := verifyLocalChecksum(localPath, info.Checksums); err != nil {
			return written, err
		}
	}

	e.emit(Event{Path: localPath, Transferred: offset + written, Total: info.Size, Done: true})
	return written, nil
}

// DownloadTree fetches a the server directory tree.
//
// When the server has an archiver and the tree fits its limits, the whole tree
// comes down as one stream. The alternative: PROPFIND the tree, then one GET
// per file: costs a round trip per file, which is what dominates the time for
// a directory of many small files.
func (e *Engine) DownloadTree(ctx context.Context, remoteRoot, localRoot string) (*Stats, error) {
	start := time.Now()

	info, err := e.c.Stat(ctx, remoteRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir {
		stats := &Stats{}
		n, err := e.DownloadFile(ctx, remoteRoot, localRoot)
		if errors.Is(err, errSkipped) {
			stats.Skipped++
			stats.Duration = time.Since(start)
			return stats, nil
		}
		if err != nil {
			return nil, err
		}
		stats.Files, stats.Bytes, stats.Duration = 1, n, time.Since(start)
		return stats, nil
	}

	if e.opts.Archive && !e.opts.DryRun {
		stats, err := e.archiveDownload(ctx, remoteRoot, localRoot)
		if err == nil {
			stats.Duration = time.Since(start)
			return stats, nil
		}
		// Falling back is the right move: the archiver may be absent, or the
		// tree may exceed its limits, and neither should fail the command.
		e.emit(Event{Path: remoteRoot, Err: err})
	}

	return e.walkDownload(ctx, remoteRoot, localRoot, start)
}

func (e *Engine) walkDownload(ctx context.Context, remoteRoot, localRoot string, start time.Time) (*Stats, error) {
	stats := &Stats{}

	type job struct{ remote, local string }
	var jobs []job

	err := e.c.Walk(ctx, remoteRoot, func(info ResourceInfo) error {
		rel := strings.TrimPrefix(info.Path, remoteRoot)
		rel = strings.TrimPrefix(rel, "/")
		local := localRoot
		if rel != "" {
			local = filepath.Join(localRoot, filepath.FromSlash(rel))
		}
		if info.IsDir {
			stats.Dirs++
			if e.opts.DryRun {
				return nil
			}
			return os.MkdirAll(local, 0o755)
		}
		jobs = append(jobs, job{remote: info.Path, local: local})
		return nil
	})
	if err != nil {
		return nil, err
	}

	var mu sync.Mutex
	err = e.eachParallel(ctx, len(jobs), func(ctx context.Context, i int) error {
		n, err := e.DownloadFile(ctx, jobs[i].remote, jobs[i].local)
		mu.Lock()
		defer mu.Unlock()
		switch {
		case errors.Is(err, errSkipped):
			stats.Skipped++
			return nil
		case err != nil:
			return err
		default:
			stats.Files++
			stats.Bytes += n
			return nil
		}
	})
	stats.Duration = time.Since(start)
	return stats, err
}

// -- helpers ------------------------------------------------------------------

// eachParallel runs fn for each index with at most Jobs in flight, stopping
// early on the first error.
func (e *Engine) eachParallel(ctx context.Context, n int, fn func(context.Context, int) error) error {
	if n == 0 {
		return nil
	}
	workers := min(e.opts.Jobs, n)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	indices := make(chan int)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error

	for range workers {
		wg.Go(func() {
			for i := range indices {
				if err := fn(ctx, i); err != nil {
					once.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
			}
		})
	}

send:
	for i := range n {
		select {
		case indices <- i:
		case <-ctx.Done():
			break send
		}
	}
	close(indices)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (e *Engine) copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, name string, offset, total int64) (int64, error) {
	buf := make([]byte, 256<<10)
	var written int64
	lastEmit := time.Now()

	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := dst.Write(buf[:n]); err != nil {
				return written, cberr.Wrap(cberr.KindOther, "write", name, err)
			}
			written += int64(n)
			// Rate-limit events: a progress bar redrawn per 256 KiB read is
			// wasted work on a fast link.
			if time.Since(lastEmit) > 100*time.Millisecond {
				e.emit(Event{Path: name, Transferred: offset + written, Total: total})
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, cberr.Wrap(cberr.KindOther, "download", name, readErr)
		}
	}
}

// walkLocal returns directories shallowest-first and files, both relative to
// root.
func walkLocal(root string) (dirs, files []string, err error) {
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
		if d.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		// Symlinks are not followed: copying the target under the link's name
		// silently changes the shape of what the user asked to transfer.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, nil, cberr.Wrap(cberr.KindOther, "read", root, err)
	}
	sort.Slice(dirs, func(i, j int) bool {
		di, dj := strings.Count(dirs[i], string(os.PathSeparator)), strings.Count(dirs[j], string(os.PathSeparator))
		if di != dj {
			return di < dj
		}
		return dirs[i] < dirs[j]
	})
	sort.Strings(files)
	return dirs, files, nil
}

// fileChecksum computes a checksum in the algorithm the server prefers,
// formatted the way OC-Checksum expects.
func (e *Engine) fileChecksum(ctx context.Context, localPath string) (string, error) {
	caps := e.c.caps(ctx)
	if len(caps.Checksums) == 0 {
		return "", nil
	}
	algo := caps.Checksums[0]
	sum, err := checksumFile(localPath, algo)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(algo) + ":" + sum, nil
}

func newHash(algo string) (hash.Hash, error) {
	switch strings.ToLower(algo) {
	case "sha1":
		return sha1.New(), nil
	case "md5":
		return md5.New(), nil
	case "adler32":
		return adler32.New(), nil
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm %q", algo)
	}
}

func checksumFile(localPath, algo string) (string, error) {
	h, err := newHash(algo)
	if err != nil {
		return "", err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return "", cberr.Wrap(cberr.KindOther, "read", localPath, err)
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return "", cberr.Wrap(cberr.KindOther, "read", localPath, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// verifyLocalChecksum checks a downloaded file against the server's checksums.
// Only algorithms both sides know are compared; a server that reports none
// leaves the file unverified rather than failing.
func verifyLocalChecksum(localPath string, serverSums map[string]string) error {
	for _, algo := range []string{"sha1", "md5", "adler32"} {
		want, ok := serverSums[algo]
		if !ok || want == "" {
			continue
		}
		got, err := checksumFile(localPath, algo)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, want) {
			return cberr.New(cberr.KindOther, "verify", localPath,
				fmt.Sprintf("checksum mismatch: the server reported %s %s but the downloaded file is %s", algo, want, got))
		}
		return nil
	}
	return nil
}
