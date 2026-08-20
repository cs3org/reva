package bundler

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"path"
	"path/filepath"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/v3/pkg/storage/utils/downloader"
	"github.com/cs3org/reva/v3/pkg/storage/utils/walker"
)

/* Formats */

// Format selects the archive format produced by Create
type Format string

// Supported archive formats
const (
	FormatTar Format = "tar"
	FormatZip Format = "zip"
	FormatTgz Format = "tgz"
)

/* Options */

// Unlimited disables the MaxNumFiles or MaxTotalSize check
const Unlimited int64 = math.MaxInt64

// Options are the per-call parameters of Create
type Options struct {
	// Roots are the resource paths to bundle
	Roots []string
	// Format selects tar, zip or tgz output
	Format Format

	// MaxNumFiles caps the number of entries, directories included
	MaxNumFiles int64
	// MaxTotalSize caps the cumulative uncompressed size of all files
	MaxTotalSize int64
	// MaxPartSize sets the maximum compressed size a part can have, 0 disables rotation
	MaxPartSize uint64

	// ZipMethod is the zip entry encoding, zip.Store or zip.Deflate
	ZipMethod uint16

	// OnEntryError is called for an entry that failed before any of its bytes were written,
	// the entry is dropped from the archive when it returns nil
	OnEntryError func(path string, err error) error
}

// entryError applies the caller's policy to an error raised before the entry was written
func (o Options) entryError(path string, err error) error {
	if o.OnEntryError == nil {
		return err
	}
	return o.OnEntryError(path, err)
}

/* Errors */

// ErrMaxFileCount is the error returned when the max files count specified in the config is reached
type ErrMaxFileCount struct{}

// ErrMaxSize is the error returned when the max total files size specified in the config is reached
type ErrMaxSize struct{}

// ErrEmptyList is the error returned when an empty list is passed when an archiver is created
type ErrEmptyList struct{}

// ErrUnsupportedFormat is the error returned when Options.Format is not a supported archive format
type ErrUnsupportedFormat struct {
	Format Format
}

// Error returns the string error msg for ErrMaxFileCount
func (ErrMaxFileCount) Error() string {
	return "bundler: reached max files count"
}

// Error returns the string error msg for ErrMaxSize
func (ErrMaxSize) Error() string {
	return "bundler: reached max total files size"
}

// Error returns the string error msg for ErrEmptyList
func (ErrEmptyList) Error() string {
	return "bundler: list of files to archive empty"
}

// Error returns the string error msg for ErrUnsupportedFormat
func (e ErrUnsupportedFormat) Error() string {
	return fmt.Sprintf("bundler: %s is not a supported archive format", e.Format)
}

/* Destinations */

// Part is one archive's destination, written then closed on completion or aborted on mid-stream failure
type Part interface {
	io.Writer
	Close() error
	Abort(err error) error
}

// PartFunc produces the destination for the part with the given 0-based index
type PartFunc func(index int) (Part, error)

// singlePart adapts a plain writer into a Part with no-op lifecycle
type singlePart struct {
	io.Writer
}

func (singlePart) Close() error          { return nil }
func (singlePart) Abort(err error) error { return nil }

// SinglePart adapts a plain writer for non-rotating use
func SinglePart(dst io.Writer) PartFunc {
	return func(int) (Part, error) {
		return singlePart{dst}, nil
	}
}

/* Bundler */

// Bundler creates archives from storage resources
type Bundler struct {
	walker     walker.Walker
	downloader downloader.Downloader
}

// New creates a Bundler walking and downloading through the given clients
func New(w walker.Walker, d downloader.Downloader) *Bundler {
	return &Bundler{walker: w, downloader: d}
}

// Create walks the roots and streams archives into the parts, rotating when MaxPartSize would be exceeded
func (b *Bundler) Create(ctx context.Context, opts Options, newPart PartFunc) error {
	if len(opts.Roots) == 0 {
		return ErrEmptyList{}
	}
	switch opts.Format {
	case FormatTar, FormatZip, FormatTgz:
	default:
		return ErrUnsupportedFormat{Format: opts.Format}
	}

	// Entry names are relative to the deepest common parent of the roots
	baseDir := DeepestCommonDir(opts.Roots)
	if pathIn(opts.Roots, baseDir) {
		// Step up when that parent is a root itself, so the root keeps its own name in the archive
		baseDir = filepath.Dir(baseDir)
	}

	// Setup per-part streaming state
	var (
		part      Part
		cw        *countingWriter
		aw        archiveWriter
		partIndex = 0
	)

	// Start a fresh part and stack the counting and archive writers on it
	openPart := func() error {
		p, err := newPart(partIndex)
		if err != nil {
			return err
		}
		part = p
		cw = &countingWriter{w: p}
		aw = newArchiveWriter(opts.Format, opts.ZipMethod, cw)
		return nil
	}

	// Finalize the archive writer then the part, clearing it so it is never finalized twice
	closePart := func() error {
		p := part
		part = nil
		if err := aw.Close(); err != nil {
			_ = p.Abort(err)
			return err
		}
		return p.Close()
	}

	// Start the first part
	if err := openPart(); err != nil {
		return err
	}

	var filesCount, sizeFiles int64

	// Create the archives by walking the specified roots
	var walkErr error
	for _, root := range opts.Roots {
		walkErr = b.walker.Walk(ctx, root, func(currentPath string, info *provider.ResourceInfo, err error) error {
			if err != nil {
				// The root comes with no info to carry on with, so its failure is always fatal
				if currentPath == root {
					return err
				}
				return opts.entryError(currentPath, err)
			}

			isDir := info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER

			// Enforce the entry count and total size limits
			filesCount++
			if filesCount > opts.MaxNumFiles {
				return ErrMaxFileCount{}
			}
			if !isDir {
				// Only count the size of files as directory sizes may be recursively computed
				sizeFiles += int64(info.Size)
				if sizeFiles > opts.MaxTotalSize {
					return ErrMaxSize{}
				}
			}

			// Get the entry name of the current resource
			fileName, err := filepath.Rel(baseDir, currentPath)
			if err != nil {
				return err
			}
			fileName = filepath.ToSlash(fileName)
			if fileName == "" {
				return nil
			}

			// Cut the part if adding the current file could exceed MaxPartSize
			if !isDir && opts.MaxPartSize > 0 && cw.n > 0 && cw.n+info.Size > opts.MaxPartSize {
				if err := closePart(); err != nil {
					return err
				}
				partIndex++
				if err := openPart(); err != nil {
					return err
				}
			}

			mtime := time.Unix(int64(info.Mtime.Seconds), 0)

			if isDir {
				return aw.WriteDir(fileName, mtime)
			}

			// Download the file content and stream it into the archive, always closing the reader
			r, err := b.downloader.Download(ctx, currentPath, "")
			if err != nil {
				return opts.entryError(currentPath, err)
			}
			writeErr := aw.WriteFile(fileName, int64(info.Size), mtime, r)
			_ = r.Close()
			return writeErr
		})
		if walkErr != nil {
			break
		}
	}
	if walkErr != nil {
		// Abort the in-flight part, if any
		if part != nil {
			_ = part.Abort(walkErr)
		}
		return walkErr
	}

	// Finalize the last part
	return closePart()
}

/* Format writers */

// archiveWriter writes directory and file entries of one archive
type archiveWriter interface {
	WriteDir(name string, mtime time.Time) error
	WriteFile(name string, size int64, mtime time.Time, content io.Reader) error
	Close() error
}

func newArchiveWriter(f Format, zipMethod uint16, dst io.Writer) archiveWriter {
	switch f {
	case FormatTar:
		return &tarWriter{tw: tar.NewWriter(dst)}
	case FormatTgz:
		gw := gzip.NewWriter(dst)
		return &tgzWriter{gw: gw, tw: tar.NewWriter(gw)}
	case FormatZip:
		return &zipWriter{w: zip.NewWriter(dst), method: zipMethod}
	default:
		// Unreachable as Create validates the format upfront
		panic(fmt.Sprintf("bundler: unsupported format %q", f))
	}
}

// tarWriter writes a plain tar archive
type tarWriter struct {
	tw *tar.Writer
}

func (t *tarWriter) WriteDir(name string, mtime time.Time) error {
	return t.tw.WriteHeader(&tar.Header{
		Name:     name,
		ModTime:  mtime,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	})
}

func (t *tarWriter) WriteFile(name string, size int64, mtime time.Time, content io.Reader) error {
	err := t.tw.WriteHeader(&tar.Header{
		Name:     name,
		ModTime:  mtime,
		Mode:     0644,
		Typeflag: tar.TypeReg,
		Size:     size,
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(t.tw, content)
	return err
}

func (t *tarWriter) Close() error {
	return t.tw.Close()
}

// tgzWriter writes a gzip-compressed tar archive, flushing per entry to keep the rotation byte count honest
type tgzWriter struct {
	gw *gzip.Writer
	tw *tar.Writer
}

func (t *tgzWriter) WriteDir(name string, mtime time.Time) error {
	err := t.tw.WriteHeader(&tar.Header{
		Name:     name,
		ModTime:  mtime,
		Mode:     0755,
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		return err
	}
	return t.gw.Flush()
}

func (t *tgzWriter) WriteFile(name string, size int64, mtime time.Time, content io.Reader) error {
	err := t.tw.WriteHeader(&tar.Header{
		Name:     name,
		ModTime:  mtime,
		Mode:     0644,
		Typeflag: tar.TypeReg,
		Size:     size,
	})
	if err != nil {
		return err
	}
	if _, err := io.Copy(t.tw, content); err != nil {
		return err
	}
	return t.gw.Flush()
}

// Close finalizes the tar stream then the gzip stream, reporting the first failure
func (t *tgzWriter) Close() error {
	if err := t.tw.Close(); err != nil {
		return err
	}
	return t.gw.Close()
}

// zipWriter writes a zip archive
type zipWriter struct {
	w      *zip.Writer
	method uint16
}

func (z *zipWriter) WriteDir(name string, mtime time.Time) error {
	_, err := z.w.CreateHeader(&zip.FileHeader{
		Name:     name + "/",
		Modified: mtime,
		Method:   z.method,
	})
	return err
}

func (z *zipWriter) WriteFile(name string, size int64, mtime time.Time, content io.Reader) error {
	dst, err := z.w.CreateHeader(&zip.FileHeader{
		Name:               name,
		Modified:           mtime,
		Method:             z.method,
		UncompressedSize64: uint64(size),
	})
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, content)
	return err
}

func (z *zipWriter) Close() error {
	return z.w.Close()
}

/* Helpers */

// countingWriter counts the bytes written through it to measure the actual archive size
type countingWriter struct {
	w io.Writer
	n uint64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.n += uint64(n)
	return n, err
}

// DeepestCommonDir returns the deepest directory common to all the given paths
func DeepestCommonDir(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// Find the maximum common substring from left
	res := path.Clean(files[0]) + "/"

	for _, file := range files[1:] {
		file = path.Clean(file) + "/"

		if len(file) < len(res) {
			res, file = file, res
		}

		for i := 0; i < len(res); i++ {
			if res[i] != file[i] {
				res = res[:i]
			}
		}
	}

	// The common substring could be between two / - inside a file name
	for i := len(res) - 1; i >= 0; i-- {
		if res[i] == '/' {
			res = res[:i+1]
			break
		}
	}
	return filepath.Clean(res)
}

// pathIn verifies that the path `f` is in the `files` list
func pathIn(files []string, f string) bool {
	f = filepath.Clean(f)
	for _, file := range files {
		if filepath.Clean(file) == f {
			return true
		}
	}
	return false
}
