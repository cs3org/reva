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
	"fmt"
	"github.com/cs3org/reva/v3/internal/cli/cberr"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// trashMultistatus renders a trash listing the way reva does: the bin itself
// first, then one entry per deleted item, each keyed in its href.
func trashMultistatus(user string, items ...trashFixture) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">`)

	// The bin itself, which a Depth:1 listing always includes.
	sb.WriteString(`<d:response><d:href>` + davTrashPrefix + "/" + user + `/</d:href>`)
	sb.WriteString(`<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>`)
	sb.WriteString(`<d:resourcetype><d:collection/></d:resourcetype>`)
	sb.WriteString(`</d:prop></d:propstat></d:response>`)

	for _, it := range items {
		href := davTrashPrefix + "/" + user + "/" + url.PathEscape(it.Key)
		if it.IsDir {
			href += "/"
		}
		sb.WriteString(`<d:response><d:href>` + href + `</d:href>`)
		sb.WriteString(`<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>`)
		sb.WriteString(`<d:displayname>` + escapeXML(it.Name) + `</d:displayname>`)
		if it.IsDir {
			sb.WriteString(`<d:resourcetype><d:collection/></d:resourcetype>`)
		} else {
			sb.WriteString(`<d:resourcetype></d:resourcetype>`)
			fmt.Fprintf(&sb, "<d:getcontentlength>%d</d:getcontentlength>", it.Size)
		}
		fmt.Fprintf(&sb, "<oc:size>%d</oc:size>", it.Size)
		sb.WriteString(`<oc:trashbin-original-filename>` + escapeXML(it.Name) + `</oc:trashbin-original-filename>`)
		sb.WriteString(`<oc:trashbin-original-location>` + escapeXML(it.Location) + `</oc:trashbin-original-location>`)
		fmt.Fprintf(&sb, "<oc:trashbin-delete-timestamp>%d</oc:trashbin-delete-timestamp>", it.Deleted)
		sb.WriteString(`</d:prop></d:propstat></d:response>`)
	}
	sb.WriteString(`</d:multistatus>`)
	return sb.String()
}

type trashFixture struct {
	Key      string
	Name     string
	Location string
	Size     int64
	Deleted  int64
	IsDir    bool
}

func TestListTrash(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-1", Name: "notes.txt", Location: "Documents/notes.txt", Size: 120, Deleted: 1767225600},
			trashFixture{Key: "key-2", Name: "old", Location: "Documents/old", Size: 4096, Deleted: 1767139200, IsDir: true},
		))
	})

	listing, err := f.client().ListTrash(context.Background(), "", TrashWindow{})
	if err != nil {
		t.Fatal(err)
	}

	// The bin itself must not appear as a restorable item.
	if len(listing.Items) != 2 {
		t.Fatalf("got %d items, want 2 (the bin itself must be excluded): %+v", len(listing.Items), listing.Items)
	}

	first := listing.Items[0]
	if first.Key != "key-1" {
		t.Errorf("Key = %q", first.Key)
	}
	if first.Name != "notes.txt" {
		t.Errorf("Name = %q", first.Name)
	}
	// The original location is what lets a user tell two deleted "notes.txt"
	// apart, so it must survive the round trip.
	if first.OriginalPath != "Documents/notes.txt" {
		t.Errorf("OriginalPath = %q", first.OriginalPath)
	}
	if first.Size != 120 {
		t.Errorf("Size = %d", first.Size)
	}
	if !first.DeletedAt.Equal(time.Unix(1767225600, 0)) {
		t.Errorf("DeletedAt = %v", first.DeletedAt)
	}
	if first.IsDir {
		t.Error("a file was reported as a directory")
	}
	if !listing.Items[1].IsDir {
		t.Error("a directory was reported as a file")
	}
}

func TestListTrashWithSpace(t *testing.T) {
	f := newFakeServer(t)
	var gotQuery string
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein"))
	})

	if _, err := f.client().ListTrash(context.Background(), "/eos/project/c/cernbox", TrashWindow{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotQuery, "base_path=") {
		t.Errorf("query = %q, want the bin selected with base_path", gotQuery)
	}
	if decoded, _ := url.QueryUnescape(gotQuery); !strings.Contains(decoded, "/eos/project/c/cernbox") {
		t.Errorf("query = %q, want the space path", decoded)
	}
}

func TestTrashKeyFromHref(t *testing.T) {
	tests := []struct {
		href string
		want string
	}{
		{davTrashPrefix + "/einstein/key-1", "key-1"},
		{davTrashPrefix + "/einstein/key-2/", "key-2"},
		{davTrashPrefix + "/einstein/", ""},
		{davTrashPrefix + "/einstein", ""},
		{"/somewhere/else", ""},
	}
	for _, tt := range tests {
		if got := trashKeyFromHref(tt.href); got != tt.want {
			t.Errorf("trashKeyFromHref(%q) = %q, want %q", tt.href, got, tt.want)
		}
	}
}

// TestRestoreTrashToOriginalLocation: the server has no "put it back": a
// restore is a MOVE, and a MOVE with no Destination is a 400. Restoring in
// place therefore means looking the original location up in the bin first.
func TestRestoreTrashToOriginalLocation(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-1", Name: "notes.txt", Location: "Documents/notes.txt", Size: 120, Deleted: 1767225600},
		))
	})
	f.on(MethodMove, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	if err := f.client().RestoreTrash(context.Background(), "key-1", ""); err != nil {
		t.Fatal(err)
	}

	req := f.lastRequest(MethodMove)
	// Rooted at the space, not at "/": the server reports the location
	// relative to the space the bin belongs to.
	if got := req.Header.Get("Destination"); !strings.HasSuffix(got, "/eos/user/e/einstein/Documents/notes.txt") {
		t.Errorf("Destination = %q, want the location rooted at the personal space", got)
	}
	if !strings.HasSuffix(req.Path, "/key-1") {
		t.Errorf("request path = %q", req.Path)
	}
	// Restoring must not silently clobber whatever is at that path now.
	if got := req.Header.Get("Overwrite"); got != "F" {
		t.Errorf("Overwrite = %q, want F", got)
	}
}

// TestRestoreTrashUnknownKey: without the listing there is no destination to
// send, and a blind MOVE would just be a 400 from the server.
func TestRestoreTrashUnknownKey(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein"))
	})

	err := f.client().RestoreTrash(context.Background(), "nope", "")
	if err == nil {
		t.Fatal("restoring a key that is not in the bin should fail")
	}
	if cberr.KindOf(err) != cberr.KindNotFound {
		t.Errorf("kind = %v, want not found", cberr.KindOf(err))
	}
}

func TestPurgeTrashItem(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodDelete, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if err := f.client().PurgeTrash(context.Background(), "key-1", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(f.lastRequest(http.MethodDelete).Path, "/key-1") {
		t.Errorf("purge path = %q", f.lastRequest(http.MethodDelete).Path)
	}
}

// TestPurgeTrashRefusesAnEmptyKey: a DELETE with no key is a request to empty
// the bin, which reaches EmptyRecycle: unimplemented in the EOS driver, so it
// answers with an internal error rather than emptying anything. Refusing here
// keeps that request from ever being sent.
func TestPurgeTrashRefusesAnEmptyKey(t *testing.T) {
	f := newFakeServer(t)
	var reached bool
	f.on(http.MethodDelete, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})

	if err := f.client().PurgeTrash(context.Background(), "", ""); err == nil {
		t.Error("purging with no key should be refused")
	}
	if reached {
		t.Error("the request was sent to the server anyway")
	}
}

// TestListTrashSendsARangeTheServerCanParse guards the whole feature against
// the way it would fail silently. ocdav discards a range it cannot parse and
// substitutes its own two days, so a wrong layout: or a '+' offset arriving as
// a space: does not produce an error anybody would notice: it produces a
// listing that ignores --since.
func TestListTrashSendsARangeTheServerCanParse(t *testing.T) {
	f := newFakeServer(t)
	var gotFrom, gotTo string
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		gotFrom, gotTo = r.URL.Query().Get("from"), r.URL.Query().Get("to")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein"))
	})

	to := time.Now()
	from := to.AddDate(0, 0, -7)
	if _, err := f.client().ListTrash(context.Background(), "", TrashWindow{From: from, To: to}); err != nil {
		t.Fatal(err)
	}

	// The layouts are reva's own, from ocs/conversions.ParseTimestamp.
	for name, got := range map[string]string{"from": gotFrom, "to": gotTo} {
		if got == "" {
			t.Fatalf("%s was not sent at all", name)
		}
		if _, err := time.Parse("2006-01-02T15:04:05Z0700", got); err != nil {
			t.Errorf("the server cannot parse %s=%q: %v", name, got, err)
		}
	}
	if !strings.Contains(gotFrom, "T") || strings.Contains(gotFrom, " ") {
		t.Errorf("from=%q reached the server mangled", gotFrom)
	}
}

// TestListTrashSplitsARangeTheServerRefuses: the storage rejects a range wider
// than max_days_in_recycle_list outright, and the width it will accept is a
// deployment's choice. Asking for less on refusal is what makes --since 90d work
// without the client having to know that number.
func TestListTrashSplitsARangeTheServerRefuses(t *testing.T) {
	f := newFakeServer(t)
	var requests int
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		requests++
		from, _ := time.Parse("2006-01-02T15:04:05Z0700", r.URL.Query().Get("from"))
		to, _ := time.Parse("2006-01-02T15:04:05Z0700", r.URL.Query().Get("to"))
		if to.Sub(from) > 14*24*time.Hour {
			http.Error(w, "too many days requested in listing the recycle bin", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-" + from.Format("0102"), Name: "notes.txt",
				Location: "Documents/notes.txt", Size: 1, Deleted: from.Unix()},
		))
	})

	to := time.Now()
	listing, err := f.client().ListTrash(context.Background(), "",
		TrashWindow{From: to.AddDate(0, 0, -60), To: to})
	if err != nil {
		t.Fatalf("a range the server refuses must be split, not returned as an error: %v", err)
	}
	if requests < 2 {
		t.Errorf("made %d requests: the refused range was never split", requests)
	}
	if len(listing.Items) == 0 {
		t.Error("the split produced no items")
	}
	if len(listing.Gaps) != 0 {
		t.Errorf("a range that splits cleanly should leave no gaps: %+v", listing.Gaps)
	}
	// Adjacent halves share the day they were split on.
	seen := map[string]bool{}
	for _, it := range listing.Items {
		if seen[it.Key] {
			t.Errorf("key %q appears twice: the shared boundary day was not deduplicated", it.Key)
		}
		seen[it.Key] = true
	}
}

// TestListTrashReportsADayItCannotList: one day holding more deletions than the
// storage will return fails that day and no more. The rest of the window is
// still worth showing: as long as the hole is declared, because a listing that
// quietly omits a day is worse than one that fails.
func TestListTrashReportsADayItCannotList(t *testing.T) {
	f := newFakeServer(t)
	busy := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		from, _ := time.Parse("2006-01-02T15:04:05Z0700", r.URL.Query().Get("from"))
		to, _ := time.Parse("2006-01-02T15:04:05Z0700", r.URL.Query().Get("to"))
		// Refuse any range touching the busy day, however narrow.
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if d.Format("2006-01-02") == busy {
				http.Error(w, "too many entries found in listing the recycle bin", http.StatusBadRequest)
				return
			}
		}
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-" + from.Format("0102"), Name: "kept.txt",
				Location: "kept.txt", Size: 1, Deleted: from.Unix()},
		))
	})

	to := time.Now()
	listing, err := f.client().ListTrash(context.Background(), "",
		TrashWindow{From: to.AddDate(0, 0, -7), To: to})
	if err != nil {
		t.Fatalf("one unlistable day must not fail the whole listing: %v", err)
	}
	if len(listing.Gaps) == 0 {
		t.Fatal("the unlistable day was not reported as a gap")
	}
	if len(listing.Items) == 0 {
		t.Error("the listable days returned nothing")
	}
}

// TestListTrashPropagatesRealFailures: only the too-much refusal is worth
// splitting. Halving a rejected credential would turn one 401 into a burst of
// them and still fail.
func TestListTrashPropagatesRealFailures(t *testing.T) {
	f := newFakeServer(t)
	var requests int
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "nope", http.StatusForbidden)
	})

	to := time.Now()
	_, err := f.client().ListTrash(context.Background(), "", TrashWindow{From: to.AddDate(0, 0, -60), To: to})
	if err == nil {
		t.Fatal("a 403 must fail the listing")
	}
	if requests != 1 {
		t.Errorf("made %d requests: a 403 was retried by splitting", requests)
	}
}

// TestRestoreTrashFindsAnItemOlderThanTheDefaultWindow is the bug this window
// work exists to fix. A restore needs the original location, which is only in
// the listing, and the listing only reaches two days back: so restoring a
// week-old file failed with "no item with this key", which reads as a bad key
// rather than a window too narrow to contain it.
func TestRestoreTrashFindsAnItemOlderThanTheDefaultWindow(t *testing.T) {
	f := newFakeServer(t)
	deleted := time.Now().AddDate(0, 0, -20)
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		from, _ := time.Parse("2006-01-02T15:04:05Z0700", r.URL.Query().Get("from"))
		w.WriteHeader(http.StatusMultiStatus)
		if deleted.Before(from) {
			fmt.Fprint(w, trashMultistatus("einstein"))
			return
		}
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-old", Name: "notes.txt", Location: "Documents/notes.txt",
				Size: 10, Deleted: deleted.Unix()},
		))
	})
	var gotDestination string
	f.on(MethodMove, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		gotDestination = r.Header.Get("Destination")
		w.WriteHeader(http.StatusCreated)
	})

	if err := f.client().RestoreTrash(context.Background(), "key-old", ""); err != nil {
		t.Fatalf("restoring a 20-day-old item failed: %v", err)
	}
	if !strings.Contains(gotDestination, "Documents/notes.txt") {
		t.Errorf("Destination = %q, want the original location", gotDestination)
	}
}

func TestTrashDeletedAtFromMilliseconds(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davTrashPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, trashMultistatus("einstein",
			trashFixture{Key: "key-1", Name: "notes.txt", Location: "Documents/notes.txt",
				Size: 10, Deleted: 1767225600000},
		))
	})

	listing, err := f.client().ListTrash(context.Background(), "", TrashWindow{})
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 {
		t.Fatalf("got %d items", len(listing.Items))
	}
	if year := listing.Items[0].DeletedAt.Year(); year != 2026 {
		t.Errorf("deleted year = %d, want 2026: the millisecond timestamp was read as seconds", year)
	}
}
