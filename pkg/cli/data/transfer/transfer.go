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

// Package transfer moves files and trees between the local disk and the server.
// Interrupted transfers resume, small files go in parallel, and the data can be
// checked with checksums.
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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

// defaultPutThreshold is the size below which a plain PUT is faster than TUS,
// which costs two extra round trips.
const defaultPutThreshold = 4 << 20

// Event reports transfer progress.
type Event struct {
	Path        string
	Transferred int64
	// Total is -1 when the size is unknown.
	Total int64
	Done  bool
	Err   error
}

// ProgressFunc receives progress events. It can be called from several
// goroutines at once.
type ProgressFunc func(Event)

// Options configures an Engine.
type Options struct {
	Jobs      int
	ChunkSize int64
	// PutThreshold is the size from which an upload is resumable.
	PutThreshold int64
	DryRun       bool
	// Overwrite replaces an existing destination. Without it, the engine skips it.
	Overwrite bool
	Verify    bool
	// Archive lets a recursive download use the archiver of the server.
	Archive  bool
	StateDir string
	Progress ProgressFunc
}

func (o *Options) applyDefaults() {
	if o.Jobs <= 0 {
		o.Jobs = min(8, max(2, runtime.NumCPU()))
	}
	if o.PutThreshold <= 0 {
		o.PutThreshold = defaultPutThreshold
	}
	if o.StateDir == "" {
		o.StateDir = DefaultStateDir("reva")
	}
}

// DefaultStateDir returns the directory of the upload state of the program
// called name.
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

// New returns an Engine on the files f and the data plane d of one server.
func New(f backend.Files, d backend.DataPlane, opts Options) *Engine {
	opts.applyDefaults()
	return &Engine{c: remote{f: f, d: d}, opts: opts}
}

func (e *Engine) emit(ev Event) {
	if e.opts.Progress != nil {
		e.opts.Progress(ev)
	}
}

// UploadFile sends one local file to remotePath.
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

// errSkipped is the error of UploadFile and DownloadFile for a skipped file.
var errSkipped = errors.New("skipped")

// simpleUpload asks the data plane to use the checksum type that the server
// prefers.
func (e *Engine) simpleUpload(ctx context.Context, localPath, remotePath string, size int64, checksum string) error {
	body := func() (io.ReadCloser, error) { return os.Open(localPath) }
	xs := ""
	if checksum != "" {
		xs = "negotiate"
	}
	return e.c.Upload(ctx, remotePath, body, size, xs)
}

// resumableUpload uses TUS and continues the recorded attempt, if there is one.
func (e *Engine) resumableUpload(ctx context.Context, localPath, remotePath string, info os.FileInfo, checksum string) (int64, error) {
	open := func() (io.ReadCloser, error) { return os.Open(localPath) }
	var last backend.Upload
	progress := func(up backend.Upload) {
		last = up
		e.saveState(localPath, remotePath, &up)
		e.emit(Event{Path: remotePath, Transferred: up.Offset, Total: info.Size()})
	}
	start := int64(0)
	var err error
	if up, ok := e.loadState(localPath, remotePath, info); ok {
		start = up.Offset
		// The recorded offset can be ahead of the server, so ResumeUpload asks it.
		err = e.c.d.ResumeUpload(ctx, *up, open, progress)
		if backend.IsNotFound(err) {
			// The server forgot the upload: start a new one.
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
		if err := stats.add(e.UploadFile(ctx, localRoot, remoteRoot)); err != nil {
			return nil, err
		}
		stats.Duration = time.Since(start)
		return stats, nil
	}

	dirs, files, err := walkLocal(localRoot)
	if err != nil {
		return nil, err
	}

	for _, rel := range dirs {
		if !e.opts.DryRun {
			if err := e.c.Mkdir(ctx, path.Join(remoteRoot, rel), true); err != nil {
				return nil, err
			}
		}
		stats.Dirs++
	}
	if e.opts.DryRun {
		stats.Dirs++
	} else if err := e.c.Mkdir(ctx, remoteRoot, true); err != nil {
		return nil, err
	}

	var mu sync.Mutex
	err = e.eachParallel(ctx, len(files), func(ctx context.Context, i int) error {
		n, err := e.UploadFile(ctx, filepath.Join(localRoot, files[i]), path.Join(remoteRoot, files[i]))
		mu.Lock()
		defer mu.Unlock()
		return stats.add(n, err)
	})
	stats.Duration = time.Since(start)
	return stats, err
}

// add counts the result of one file and returns its error, unless it was skipped.
func (s *Stats) add(n int64, err error) error {
	switch {
	case errors.Is(err, errSkipped):
		s.Skipped++
	case err != nil:
		return err
	default:
		s.Files++
		s.Bytes += n
	}
	return nil
}

// DownloadFile fetches one server file to localPath. It resumes a partial
// transfer.
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

	// Readers of the destination never see a truncated file, and an interrupted
	// transfer leaves a .part file to resume from.
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

// DownloadTree fetches a server directory tree. When the archiver can serve it,
// the tree comes as one stream, which saves a round trip per file.
func (e *Engine) DownloadTree(ctx context.Context, remoteRoot, localRoot string) (*Stats, error) {
	start := time.Now()

	info, err := e.c.Stat(ctx, remoteRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir {
		stats := &Stats{}
		if err := stats.add(e.DownloadFile(ctx, remoteRoot, localRoot)); err != nil {
			return nil, err
		}
		stats.Duration = time.Since(start)
		return stats, nil
	}

	if e.opts.Archive && !e.opts.DryRun {
		stats, err := e.archiveDownload(ctx, remoteRoot, localRoot)
		if err == nil {
			stats.Duration = time.Since(start)
			return stats, nil
		}
		// The archiver may be absent, or the tree too big for it: fall back.
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
		return stats.add(n, err)
	})
	stats.Duration = time.Since(start)
	return stats, err
}

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
			// A redraw per read is wasted work on a fast link.
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

// walkLocal returns the directories, shallowest first, and the files, as slash
// paths relative to root.
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
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			dirs = append(dirs, rel)
			return nil
		}
		// Copying the target of a link under its name would change the tree.
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, nil, cberr.Wrap(cberr.KindOther, "read", root, err)
	}
	slices.SortFunc(dirs, byDepth)
	slices.Sort(files)
	return dirs, files, nil
}

// fileChecksum returns the checksum in the first algorithm of the server, in
// the OC-Checksum format.
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

// verifyLocalChecksum compares a file with the checksums of the server. A
// server without a known checksum leaves the file unverified.
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
