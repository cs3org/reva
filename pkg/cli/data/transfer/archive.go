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
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func (e *Engine) archiveDownload(ctx context.Context, remoteRoot, localRoot string) (*Stats, error) {
	// Only tar can be extracted from a stream: zip needs random access. An empty
	// list means unknown formats, and the archiver may still be there.
	if caps := e.c.caps(ctx); len(caps.ArchiveFormats) > 0 && !slices.Contains(caps.ArchiveFormats, "tar") {
		return nil, cberr.New(cberr.KindOther, "download archive", remoteRoot,
			"the server's archiver cannot produce a tar stream")
	}

	body, err := e.c.d.Archive(ctx, []*provider.Reference{ref(remoteRoot)}, "tar")
	if err != nil {
		return nil, err
	}
	defer body.Close()

	return e.extractTar(ctx, body, localRoot, path.Base(remoteRoot))
}

// extractTar unpacks a tar stream into localRoot. It drops the leading
// directory stripRoot from each entry, if it is not empty.
func (e *Engine) extractTar(ctx context.Context, r io.Reader, localRoot, stripRoot string) (*Stats, error) {
	stats := &Stats{}

	absRoot, err := filepath.Abs(localRoot)
	if err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "write", localRoot, err)
	}
	if err := os.MkdirAll(absRoot, 0o755); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "create", localRoot, err)
	}

	tr := tar.NewReader(r)
	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			return stats, nil
		}
		if err != nil {
			return stats, cberr.Wrap(cberr.KindOther, "read archive", localRoot, err)
		}

		name := stripArchiveRoot(hdr.Name, stripRoot)
		if name == "" {
			continue
		}

		// Do not trust the entry names: they must stay inside the destination.
		target, err := safeJoin(absRoot, name)
		if err != nil {
			return stats, err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return stats, cberr.Wrap(cberr.KindOther, "create", target, err)
			}
			stats.Dirs++
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return stats, cberr.Wrap(cberr.KindOther, "create", filepath.Dir(target), err)
			}
			if !e.opts.Overwrite {
				if _, err := os.Stat(target); err == nil {
					stats.Skipped++
					if _, err := io.Copy(io.Discard, tr); err != nil {
						return stats, cberr.Wrap(cberr.KindOther, "read archive", target, err)
					}
					continue
				}
			}
			n, err := writeArchiveFile(target, tr)
			if err != nil {
				return stats, err
			}
			stats.Files++
			stats.Bytes += n
			e.emit(Event{Path: target, Transferred: n, Total: hdr.Size, Done: true})

		default:
			// A link could point outside the destination, so skip links and
			// every other type.
			stats.Skipped++
		}
	}
}

func writeArchiveFile(target string, r io.Reader) (int64, error) {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, cberr.Wrap(cberr.KindOther, "create", target, err)
	}
	defer f.Close()

	n, err := io.Copy(f, r)
	if err != nil {
		return n, cberr.Wrap(cberr.KindOther, "write", target, err)
	}
	return n, nil
}

// safeJoin resolves name against root and refuses a name that escapes it.
// It uses IsLocal because on Windows more names than ".." and absolute paths
// escape: "C:x", "a:b" (a data stream) and "NUL" (a device).
func safeJoin(root, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if !filepath.IsLocal(cleaned) {
		return "", cberr.New(cberr.KindOther, "extract archive", name,
			"the archive contains an entry that would be written outside the destination")
	}
	return filepath.Join(root, cleaned), nil
}

// stripArchiveRoot normalises an entry name and removes the wrapper directory.
// The archiver wraps the entries in a directory named after the request
// ("tree/a.txt"), but localRoot already stands for it. The wrapper itself
// becomes empty.
func stripArchiveRoot(name, stripRoot string) string {
	name = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, "\\", "/")), "/")
	if name == "." {
		return ""
	}
	if stripRoot == "" || stripRoot == "." || stripRoot == "/" {
		return name
	}
	if name == stripRoot {
		return ""
	}
	return strings.TrimPrefix(name, stripRoot+"/")
}
