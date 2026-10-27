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
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Version is one revision of a file.
type Version struct {
	// Key identifies the version. The CLI treats it as opaque.
	Key      string    `json:"key"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	ETag     string    `json:"etag,omitempty"`
}

func (c *Client) versionsURL(resourceID, key string) string {
	u := c.URL(davMetaPrefix + "/" + url.PathEscape(resourceID) + "/v")
	if key != "" {
		u += "/" + url.PathEscape(key)
	}
	return u
}

// ListVersions returns the revisions of a file, newest first. resourceID is
// the persistent id from Stat, not a path: the versions follow a rename.
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
			// The "." entry is the file itself, not a revision.
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
			// reva keys a version by its modification time.
			v.Modified = epochToTime(n)
		}
		versions = append(versions, v)
	}

	slices.SortStableFunc(versions, func(a, b Version) int { return b.Modified.Compare(a.Modified) })
	return versions, nil
}

// versionKeyFromHref returns the part after the "/v/" segment.
func versionKeyFromHref(href string) string {
	_, after, ok := strings.Cut(href, "/v/")
	if !ok {
		return ""
	}
	return strings.Trim(after, "/")
}

// RestoreVersion makes a revision current. The revision that it replaces
// becomes a version itself.
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

// epochToTime reads an epoch in seconds or in milliseconds. Storage drivers
// differ, and a value past 1e11 seconds would be the year 5138.
func epochToTime(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	if n >= 1e11 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
