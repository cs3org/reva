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
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Version is one historical revision of a file.
type Version struct {
	// Key identifies the version. reva uses the modification timestamp, but the
	// CLI treats it as opaque so a storage that numbers versions differently
	// still works.
	Key string `json:"key"`
	// Size is the byte count of that revision.
	Size int64 `json:"size"`
	// Modified is when that revision was current.
	Modified time.Time `json:"modified"`
	// ETag identifies the revision's content.
	ETag string `json:"etag,omitempty"`
}

// versionsURL builds the meta URL for a resource's version list.
func (c *Client) versionsURL(resourceID, key string) string {
	u := c.URL(davMetaPrefix + "/" + url.PathEscape(resourceID) + "/v")
	if key != "" {
		u += "/" + url.PathEscape(key)
	}
	return u
}

// ListVersions returns the historical revisions of a file, newest first.
//
// resourceID is the persistent id from Stat, not a path: versions survive a
// rename, so addressing them by path would lose the history the moment a file
// moved.
func (c *Client) ListVersions(ctx context.Context, resourceID string) ([]Version, error) {
	resp, err := c.do(ctx, request{
		method: MethodPropfind,
		url:    c.versionsURL(resourceID, ""),
		header: http.Header{
			"Depth":        []string{"1"},
			"Content-Type": []string{"application/xml"},
		},
		body:    stringBody(propfindBody),
		op:      "list versions",
		path:    resourceID,
		expects: []int{http.StatusMultiStatus, http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ms multistatus
	if err := decodeXML(resp.Body, &ms, "list versions", resourceID); err != nil {
		return nil, err
	}

	versions := make([]Version, 0, len(ms.Responses))
	for _, entry := range ms.Responses {
		props, ok := okProps(entry)
		if !ok {
			continue
		}
		href, err := url.PathUnescape(entry.Href)
		if err != nil {
			href = entry.Href
		}
		key := versionKeyFromHref(href)
		if key == "" {
			// The listing includes the file itself as the "." entry; it has no
			// version key and is not a revision.
			continue
		}

		v := Version{Key: key, ETag: strings.Trim(props.GetETag, `"`)}
		if n, err := strconv.ParseInt(props.GetLength, 10, 64); err == nil {
			v.Size = n
		} else if n, err := strconv.ParseInt(props.OCSize, 10, 64); err == nil {
			v.Size = n
		}
		if t, err := http.ParseTime(props.GetLastMod); err == nil {
			v.Modified = t
		} else if n, err := strconv.ParseInt(key, 10, 64); err == nil {
			// reva keys versions by the modification time, so the key itself is
			// a usable fallback when the property is absent.
			v.Modified = epochToTime(n)
		}
		versions = append(versions, v)
	}

	// Newest first: that is the order a user scanning for "the one from this
	// morning" wants, and it matches how the web UI presents them.
	sortVersionsNewestFirst(versions)
	return versions, nil
}

func sortVersionsNewestFirst(versions []Version) {
	for i := 1; i < len(versions); i++ {
		for j := i; j > 0 && versions[j].Modified.After(versions[j-1].Modified); j-- {
			versions[j], versions[j-1] = versions[j-1], versions[j]
		}
	}
}

// versionKeyFromHref extracts the version key, which the server places after
// the "/v/" segment.
func versionKeyFromHref(href string) string {
	_, after, ok := strings.Cut(href, "/v/")
	if !ok {
		return ""
	}
	return strings.Trim(after, "/")
}

// RestoreVersion makes a historical revision current. The revision being
// replaced becomes a version in its own right, so this is not destructive.
func (c *Client) RestoreVersion(ctx context.Context, resourceID, key string) error {
	if key == "" {
		return cberr.Usagef("no version key given")
	}
	resp, err := c.do(ctx, request{
		method:  MethodCopy,
		url:     c.versionsURL(resourceID, key),
		op:      "restore a version",
		path:    key,
		expects: []int{http.StatusCreated, http.StatusNoContent, http.StatusOK},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// DownloadVersion opens a historical revision for reading, leaving the current
// one untouched. The caller must close the returned ReadCloser.
func (c *Client) DownloadVersion(ctx context.Context, resourceID, key string) (io.ReadCloser, int64, error) {
	if key == "" {
		return nil, 0, cberr.Usagef("no version key given")
	}
	resp, err := c.do(ctx, request{
		method:  http.MethodGet,
		url:     c.versionsURL(resourceID, key),
		op:      "download a version",
		path:    key,
		expects: []int{http.StatusOK},
	})
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.ContentLength, nil
}

// msThreshold is where a plausible epoch in seconds stops. Seconds past this
// would be the year 5138, so a larger number is milliseconds.
const msThreshold = 1e11

// epochToTime converts an epoch that may be in seconds or in milliseconds.
//
// Storage drivers disagree: reva's trash keys and some version keys are in
// milliseconds while the CS3 timestamps are in seconds, and reading one as the
// other puts the file fifty thousand years in the future.
func epochToTime(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	if n >= msThreshold {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
