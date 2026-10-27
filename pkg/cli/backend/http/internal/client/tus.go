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
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

const (
	hdrTusResumable   = "Tus-Resumable"
	hdrUploadLength   = "Upload-Length"
	hdrUploadMetadata = "Upload-Metadata"
	tusVersion        = "1.0.0"
)

// CreateUpload starts a TUS upload for a file of the given size and returns
// the URL of the upload. The parent directory of p must exist.
func (c *Client) CreateUpload(ctx context.Context, p string, size int64, checksum string) (string, error) {
	dir, name := path.Split(cleanPath(p))
	if name == "" {
		return "", cberr.Usagef("%q is not a file path", p)
	}

	parentURL, err := c.davURL(ctx, strings.TrimSuffix(dir, "/"))
	if err != nil {
		return "", err
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
		return "", err
	}
	defer drain(resp)

	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", cberr.New(cberr.KindOther, "start upload", p,
			"the server accepted the upload but returned no Location header")
	}
	abs, err := c.absoluteURL(loc)
	if err != nil {
		return "", cberr.Wrap(cberr.KindOther, "start upload", p, err)
	}
	return abs, nil
}

// absoluteURL resolves a Location header against the base URL. The TUS spec
// allows a relative one, and a proxy can rewrite it.
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

// encodeTusMetadata renders the Upload-Metadata header. The keys are sorted,
// so the header is the same each time.
func encodeTusMetadata(meta map[string]string) string {
	parts := make([]string, 0, len(meta))
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
