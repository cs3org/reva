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

package client

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Archive formats the archiver can produce.
const (
	ArchiveTar = "tar"
	ArchiveZip = "zip"
)

// Archive streams a directory tree as a single archive.
//
// This exists because the obvious alternative: PROPFIND the tree, then issue
// one GET per file: costs a round trip per file, which dominates the transfer
// time for a directory of many small files. The archiver does the walk
// server-side and returns one stream. The caller is responsible for falling
// back to a walk when the server has no archiver or the tree exceeds the
// advertised limits.
func (c *Client) Archive(ctx context.Context, paths []string, format string) (io.ReadCloser, error) {
	u, err := c.ArchiveURL(ctx, paths, format)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, request{
		method:  http.MethodGet,
		url:     u,
		op:      "download archive",
		path:    strings.Join(paths, ", "),
		expects: []int{http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ArchiveURL returns the URL of the archiver for paths in format.
func (c *Client) ArchiveURL(ctx context.Context, paths []string, format string) (string, error) {
	caps, err := c.Capabilities(ctx)
	if err != nil {
		return "", err
	}
	if !caps.ArchiverEnabled {
		return "", cberr.New(cberr.KindOther, "download archive", strings.Join(paths, ", "),
			"this server has no archiver service")
	}
	if !caps.SupportsArchiveFormat(format) {
		return "", cberr.Usagef("this server cannot produce %q archives, it supports: %s",
			format, strings.Join(caps.ArchiverFormats, ", "))
	}
	if len(paths) == 0 {
		return "", cberr.Usagef("no paths to archive")
	}

	// The advertised archiver URL may already carry a query of its own, so
	// parse it rather than concatenating: appending to the path would bury the
	// "?" inside the path component and silently drop the server's parameters.
	ref, err := url.Parse(caps.ArchiverURL)
	if err != nil {
		return "", cberr.Wrap(cberr.KindOther, "download archive", "", err)
	}
	q := ref.Query()
	for _, p := range paths {
		q.Add("path", p)
	}
	q.Set("arch_type", format)
	ref.RawQuery = q.Encode()

	u := c.URL(ref.EscapedPath())
	if ref.RawQuery != "" {
		u += "?" + ref.RawQuery
	}
	return u, nil
}

// ArchiveFits reports whether a tree of the given file count and total size is
// within the archiver's advertised limits. A server that advertises no limit
// always fits.
func (c *Capabilities) ArchiveFits(numFiles int64, totalSize int64) bool {
	if c == nil || !c.ArchiverEnabled {
		return false
	}
	if c.ArchiverMaxNumFiles > 0 && numFiles > c.ArchiverMaxNumFiles {
		return false
	}
	if c.ArchiverMaxSize > 0 && totalSize > c.ArchiverMaxSize {
		return false
	}
	return true
}
