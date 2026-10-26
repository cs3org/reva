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
	"encoding/base64"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// TUS protocol headers.
const (
	hdrTusResumable   = "Tus-Resumable"
	hdrUploadLength   = "Upload-Length"
	hdrUploadOffset   = "Upload-Offset"
	hdrUploadMetadata = "Upload-Metadata"
	tusVersion        = "1.0.0"
)

// Upload is a resumable upload in progress. The URL is the only state needed to
// continue it, so a caller that persists it can resume across process restarts
// : which is the point: an interrupted 50 GB upload should continue, not start
// again.
type Upload struct {
	// URL is the upload endpoint returned by the server.
	URL string `json:"url"`
	// Path is the destination in CERNBox.
	Path string `json:"path"`
	// Size is the total length of the upload.
	Size int64 `json:"size"`
	// Offset is how many bytes the server has accepted so far.
	Offset int64 `json:"offset"`
}

// CreateUpload starts a resumable upload for a file of the given size. The
// destination's parent directory must exist.
func (c *Client) CreateUpload(ctx context.Context, p string, size int64, checksum string) (*Upload, error) {
	dir, name := path.Split(cleanPath(p))
	if name == "" {
		return nil, cberr.Usagef("%q is not a file path", p)
	}

	parentURL, err := c.davURL(ctx, strings.TrimSuffix(dir, "/"))
	if err != nil {
		return nil, err
	}

	header := http.Header{
		hdrTusResumable: []string{tusVersion},
		hdrUploadLength: []string{strconv.FormatInt(size, 10)},
		hdrUploadMetadata: []string{encodeTusMetadata(map[string]string{
			"filename": name,
			"dir":      strings.TrimSuffix(dir, "/"),
		})},
		"Content-Length": []string{"0"},
	}
	if checksum != "" {
		header.Set("OC-Checksum", checksum)
	}

	resp, err := c.do(ctx, request{
		method:  http.MethodPost,
		url:     parentURL,
		header:  header,
		op:      "start upload",
		path:    p,
		expects: []int{http.StatusCreated, http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	defer drain(resp)

	loc := resp.Header.Get("Location")
	if loc == "" {
		return nil, cberr.New(cberr.KindOther, "start upload", p,
			"the server accepted the upload but returned no Location header")
	}
	abs, err := c.absoluteURL(loc)
	if err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "start upload", p, err)
	}
	return &Upload{URL: abs, Path: p, Size: size}, nil
}

// UploadOffset asks the server how much of an upload it already holds. This is
// what makes resume correct rather than hopeful: the client never assumes its
// own record of progress matches the server's.
func (c *Client) UploadOffset(ctx context.Context, up *Upload) (int64, error) {
	resp, err := c.do(ctx, request{
		method:  http.MethodHead,
		url:     up.URL,
		header:  http.Header{hdrTusResumable: []string{tusVersion}},
		op:      "check upload progress",
		path:    up.Path,
		expects: []int{http.StatusOK, http.StatusNoContent},
	})
	if err != nil {
		return 0, err
	}
	defer drain(resp)

	raw := resp.Header.Get(hdrUploadOffset)
	if raw == "" {
		return 0, cberr.New(cberr.KindOther, "check upload progress", up.Path,
			"the server did not report an upload offset")
	}
	offset, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, cberr.Wrap(cberr.KindOther, "check upload progress", up.Path, err)
	}
	return offset, nil
}

// UploadChunk sends the next chunk at the given offset and returns the server's
// new offset.
func (c *Client) UploadChunk(ctx context.Context, up *Upload, offset int64, open func() (io.ReadCloser, error), length int64) (int64, error) {
	header := http.Header{
		hdrTusResumable: []string{tusVersion},
		hdrUploadOffset: []string{strconv.FormatInt(offset, 10)},
		"Content-Type":  []string{"application/offset+octet-stream"},
	}

	resp, err := c.do(ctx, request{
		method:  http.MethodPatch,
		url:     up.URL,
		header:  header,
		body:    readerBody(open, length),
		op:      "upload",
		path:    up.Path,
		expects: []int{http.StatusNoContent, http.StatusOK, http.StatusCreated},
	})
	if err != nil {
		return offset, err
	}
	defer drain(resp)

	// The server's offset is authoritative. Trusting our own arithmetic here
	// would let a partially-accepted chunk silently corrupt the file.
	if raw := resp.Header.Get(hdrUploadOffset); raw != "" {
		newOffset, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr == nil {
			return newOffset, nil
		}
	}
	return offset + length, nil
}

// absoluteURL resolves a possibly-relative Location header against the client's
// base URL. reva's datagateway returns an absolute URL, but the TUS spec allows
// a relative one and a proxy may rewrite it.
func (c *Client) absoluteURL(loc string) (string, error) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", fmt.Errorf("invalid Location %q: %w", loc, err)
	}
	if u.IsAbs() {
		return u.String(), nil
	}
	return c.base.ResolveReference(u).String(), nil
}

// encodeTusMetadata renders the Upload-Metadata header: space-separated
// key/base64-value pairs.
func encodeTusMetadata(meta map[string]string) string {
	parts := make([]string, 0, len(meta))
	// Sorted so the header is deterministic, which makes tests and server logs
	// easier to read.
	for _, k := range slices.Sorted(maps.Keys(meta)) {
		v := meta[k]
		if v == "" {
			parts = append(parts, k)
			continue
		}
		parts = append(parts, k+" "+base64.StdEncoding.EncodeToString([]byte(v)))
	}
	return strings.Join(parts, ",")
}
