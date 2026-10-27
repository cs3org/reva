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
	"errors"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// TrashItem is one deleted resource.
type TrashItem struct {
	// Key identifies the item for restore and purge. It is opaque.
	Key  string `json:"key"`
	Name string `json:"name"`
	// OriginalPath is relative to the space root.
	OriginalPath string    `json:"original_path"`
	DeletedAt    time.Time `json:"deleted_at"`
	IsDir        bool      `json:"is_dir"`
	Size         int64     `json:"size,omitempty"`
}

// TrashWindow is the span of deletion times that a listing covers. Without a
// range the storage lists only the last two days, so the caller must know the
// window it shows.
type TrashWindow struct {
	From time.Time
	To   time.Time
}

const (
	// DefaultTrashDays is the span that the storage lists by default.
	DefaultTrashDays = 2

	// MaxTrashSpanDays is the default of max_days_in_recycle_list. A deployment
	// can lower it, so a refusal is not an error.
	MaxTrashSpanDays = 14

	// trashTimeLayout is the layout that ocdav parses. It ignores a value that
	// it cannot parse and lists the default window.
	trashTimeLayout = "2006-01-02T15:04:05Z0700"
)

func (w TrashWindow) resolve(now time.Time) TrashWindow {
	if w.To.IsZero() {
		w.To = now
	}
	if w.From.IsZero() {
		w.From = w.To.AddDate(0, 0, -DefaultTrashDays)
	}
	return w
}

// TrashGap is a stretch of the window that the server would not list.
type TrashGap struct {
	From time.Time
	To   time.Time
	Err  error
}

// TrashListing is what a recycle bin returned.
type TrashListing struct {
	// Items are the deleted resources, newest first.
	Items  []TrashItem
	Window TrashWindow
	// Gaps are the stretches of Window that the server refused, when one day
	// holds more deletions than max_recycle_entries. Items covers the rest.
	Gaps []TrashGap
}

// trashPropfindBody asks for the original location, which tells apart files
// with the same name.
const trashPropfindBody = `<?xml version="1.0" encoding="UTF-8"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <d:getcontentlength/>
    <oc:size/>
    <oc:trashbin-original-filename/>
    <oc:trashbin-original-location/>
    <oc:trashbin-delete-timestamp/>
    <oc:trashbin-delete-datetime/>
  </d:prop>
</d:propfind>`

func (c *Client) trashURL(ctx context.Context, key string, q url.Values) (string, error) {
	me, err := c.Me(ctx)
	if err != nil {
		return "", err
	}
	u := c.URL(davTrashPrefix + "/" + escapePath(me.Username))
	if key != "" {
		u += "/" + url.PathEscape(key)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u, nil
}

// trashQuery selects a bin and, optionally, a range. The storage uses the range
// only when it has both ends. The values must be encoded: a raw '+' would
// arrive as a space.
func trashQuery(basePath string, from, to time.Time) url.Values {
	q := url.Values{}
	if basePath != "" {
		q.Set("base_path", basePath)
	}
	if !from.IsZero() && !to.IsZero() {
		q.Set("from", from.UTC().Format(trashTimeLayout))
		q.Set("to", to.UTC().Format(trashTimeLayout))
	}
	return q
}

// ListTrash returns the items deleted within a window. An empty basePath means
// the caller's home. A zero window means the default span.
//
// The storage refuses a range of more than max_days_in_recycle_list days, and a
// range with more than max_recycle_entries deletions. Both are answered by
// halving the range and asking again.
func (c *Client) ListTrash(ctx context.Context, basePath string, window TrashWindow) (*TrashListing, error) {
	window = window.resolve(time.Now())

	items, gaps, err := c.listTrashRange(ctx, basePath, window.From, window.To)
	if err != nil {
		return nil, err
	}
	return &TrashListing{Items: sortTrash(items), Window: window, Gaps: gaps}, nil
}

func (c *Client) listTrashRange(ctx context.Context, basePath string, from, to time.Time) ([]TrashItem, []TrashGap, error) {
	items, err := c.listTrashOnce(ctx, basePath, from, to)
	if err == nil {
		return items, nil, nil
	}
	// Split only for this refusal. Any other error would repeat in every half.
	if !isTrashRangeRefused(err) {
		return nil, nil, err
	}
	if !to.After(from.Add(24 * time.Hour)) {
		// One day and still refused: report a gap and list the rest.
		return nil, []TrashGap{{From: from, To: to, Err: err}}, nil
	}

	mid := from.Add(to.Sub(from) / 2).Truncate(24 * time.Hour)
	if !mid.After(from) || !to.After(mid) {
		mid = from.Add(24 * time.Hour)
	}
	left, leftGaps, err := c.listTrashRange(ctx, basePath, from, mid)
	if err != nil {
		return nil, nil, err
	}
	right, rightGaps, err := c.listTrashRange(ctx, basePath, mid, to)
	if err != nil {
		return nil, nil, err
	}
	return append(left, right...), append(leftGaps, rightGaps...), nil
}

// isTrashRangeRefused reports whether the server refused the size of the range.
// "Too many days" and "too many entries" both arrive as InvalidArgument, which
// ocdav renders as 400. Nothing else on this endpoint answers 400.
func isTrashRangeRefused(err error) bool {
	var e *cberr.Error
	return errors.As(err, &e) && e.Status == http.StatusBadRequest
}

func (c *Client) listTrashOnce(ctx context.Context, basePath string, from, to time.Time) ([]TrashItem, error) {
	u, err := c.trashURL(ctx, "", trashQuery(basePath, from, to))
	if err != nil {
		return nil, err
	}

	resp, err := c.do(ctx, request{
		method: MethodPropfind,
		url:    u,
		header: http.Header{
			"Depth":        []string{"1"},
			"Content-Type": []string{"application/xml"},
		},
		body:    stringBody(trashPropfindBody),
		op:      "list the trash bin",
		path:    basePath,
		expects: []int{http.StatusMultiStatus, http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ms multistatus
	if err := decodeXML(resp.Body, &ms, "list the trash bin", basePath); err != nil {
		return nil, err
	}

	items := make([]TrashItem, 0, len(ms.Responses))
	for _, entry := range ms.Responses {
		item, ok := trashEntryToItem(entry)
		if !ok {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// sortTrash puts the newest deletion first and drops repeats. The two halves of
// a split range both include the day of the split, so an item deleted that day
// comes back twice.
func sortTrash(items []TrashItem) []TrashItem {
	seen := make(map[string]struct{}, len(items))
	out := make([]TrashItem, 0, len(items))
	for _, it := range items {
		if _, dup := seen[it.Key]; dup {
			continue
		}
		seen[it.Key] = struct{}{}
		out = append(out, it)
	}
	slices.SortStableFunc(out, func(a, b TrashItem) int {
		return b.DeletedAt.Compare(a.DeletedAt)
	})
	return out
}

func trashEntryToItem(entry multistatusEntry) (TrashItem, bool) {
	props, ok := okProps(entry)
	if !ok {
		return TrashItem{}, false
	}

	href, err := url.PathUnescape(entry.Href)
	if err != nil {
		href = entry.Href
	}
	key := trashKeyFromHref(href)
	if key == "" {
		// The bin itself appears in a Depth:1 listing and has no key.
		return TrashItem{}, false
	}

	item := TrashItem{
		Key:          key,
		Name:         firstNonEmptyString(props.TrashFilename, props.DisplayName),
		OriginalPath: props.TrashLocation,
		IsDir:        props.ResourceType != nil && props.ResourceType.Collection != nil,
	}
	if n, err := strconv.ParseInt(props.OCSize, 10, 64); err == nil {
		item.Size = n
	} else if n, err := strconv.ParseInt(props.GetLength, 10, 64); err == nil {
		item.Size = n
	}
	if n, err := strconv.ParseInt(props.TrashTimestamp, 10, 64); err == nil && n > 0 {
		item.DeletedAt = epochToTime(n)
	} else if t, err := http.ParseTime(props.TrashDatetime); err == nil {
		item.DeletedAt = t
	}
	return item, true
}

// trashKeyFromHref returns the item key, which follows the username in the href.
func trashKeyFromHref(href string) string {
	_, rest, ok := strings.Cut(href, davTrashPrefix)
	if !ok {
		return ""
	}
	_, key, ok := strings.Cut(strings.Trim(rest, "/"), "/")
	if !ok {
		return ""
	}
	return strings.Trim(key, "/")
}

// RestoreTrash puts a deleted item back where it came from.
//
// A restore is a MOVE, and a MOVE needs a Destination, so the original location
// is looked up in the bin first. The caller cannot choose another place: the EOS
// driver ignores the Destination, and ocdav deletes whatever is at the
// destination first and then reports a failure.
func (c *Client) RestoreTrash(ctx context.Context, key, basePath string) error {
	src, err := c.trashURL(ctx, key, trashQuery(basePath, time.Time{}, time.Time{}))
	if err != nil {
		return err
	}

	dst, err := c.originalPathOf(ctx, key, basePath)
	if err != nil {
		return err
	}

	dstURL, err := c.davURL(ctx, dst)
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Destination", dstURL)
	header.Set("Overwrite", "F")

	resp, err := c.do(ctx, request{
		method:  MethodMove,
		url:     src,
		header:  header,
		op:      "restore from the trash bin",
		path:    key,
		expects: []int{http.StatusCreated, http.StatusNoContent, http.StatusOK},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// trashSearchDays are the spans that originalPathOf tries in order. Most
// restores are of a recent deletion, which the first span finds in one request.
var trashSearchDays = []int{DefaultTrashDays, MaxTrashSpanDays, 90}

func (c *Client) originalPathOf(ctx context.Context, key, basePath string) (string, error) {
	to := time.Now()
	for _, days := range trashSearchDays {
		listing, err := c.ListTrash(ctx, basePath, TrashWindow{From: to.AddDate(0, 0, -days), To: to})
		if err != nil {
			return "", err
		}
		for _, it := range listing.Items {
			if it.Key != key {
				continue
			}
			return c.restoreDestination(ctx, it, basePath)
		}
	}
	return "", cberr.New(cberr.KindNotFound, "restore from the trash bin", key,
		"no item with this key was found in the last 90 days of the trash bin: "+
			"'trash list --since 90d' shows what is there")
}

func (c *Client) restoreDestination(ctx context.Context, it TrashItem, basePath string) (string, error) {
	if it.OriginalPath == "" {
		return "", cberr.New(cberr.KindOther, "restore from the trash bin", it.Key,
			"the server did not say where this item came from; name a destination")
	}
	if path.IsAbs(it.OriginalPath) {
		return it.OriginalPath, nil
	}
	// The location is relative to the space root. Rooting it at "/" would aim at
	// the top of the namespace.
	root := basePath
	if root == "" {
		var err error
		root, err = c.ResolveSpace(ctx, "home")
		if err != nil {
			return "", err
		}
	}
	return path.Join(root, it.OriginalPath), nil
}

// PurgeTrash permanently deletes one item. A DELETE with no key reaches
// EmptyRecycle, which the EOS driver does not implement, so an empty key is
// refused.
func (c *Client) PurgeTrash(ctx context.Context, key, basePath string) error {
	if key == "" {
		return cberr.New(cberr.KindUsage, "purge a trash item", "",
			"no key given, and the whole bin cannot be emptied in one request")
	}
	u, err := c.trashURL(ctx, key, trashQuery(basePath, time.Time{}, time.Time{}))
	if err != nil {
		return err
	}

	resp, err := c.do(ctx, request{
		method:  http.MethodDelete,
		url:     u,
		op:      "purge a trash item",
		path:    key,
		expects: []int{http.StatusNoContent, http.StatusOK, http.StatusAccepted},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

func okProps(entry multistatusEntry) (propsRaw, bool) {
	for _, ps := range entry.Propstat {
		if strings.Contains(ps.Status, " 200 ") {
			return ps.Prop, true
		}
	}
	return propsRaw{}, false
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
