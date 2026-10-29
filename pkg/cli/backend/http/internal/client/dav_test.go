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
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

func TestStat(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{
		Path:      "/eos/user/e/einstein/notes.txt",
		Size:      1258,
		ETag:      "abc123",
		FileID:    "localhome$MFZWI...!fileid-1",
		Modified:  "Mon, 02 Jan 2006 15:04:05 GMT",
		MimeType:  "text/plain",
		Perms:     "RDNVW",
		Checksums: "SHA1:40bd001563085fc35165329ea1ff5c5ecbdbbeef MD5:202cb962ac59075b964b07152d234b70",
	})

	info, err := f.client().Stat(context.Background(), "/eos/user/e/einstein/notes.txt")
	if err != nil {
		t.Fatal(err)
	}

	if info.Path != "/eos/user/e/einstein/notes.txt" {
		t.Errorf("Path = %q", info.Path)
	}
	if info.Name != "notes.txt" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.IsDir {
		t.Error("IsDir = true for a file")
	}
	if info.Size != 1258 {
		t.Errorf("Size = %d, want 1258", info.Size)
	}
	if info.ETag != "abc123" {
		t.Errorf("ETag = %q, want the quotes stripped", info.ETag)
	}
	if info.ID != "localhome$MFZWI...!fileid-1" {
		t.Errorf("ID = %q", info.ID)
	}
	if info.Modified.IsZero() {
		t.Error("Modified was not parsed")
	}
	if info.Permissions != "RDNVW" {
		t.Errorf("Permissions = %q", info.Permissions)
	}
	if got := info.Checksums["sha1"]; got != "40bd001563085fc35165329ea1ff5c5ecbdbbeef" {
		t.Errorf("sha1 checksum = %q", got)
	}
	if got := info.Checksums["md5"]; got != "202cb962ac59075b964b07152d234b70" {
		t.Errorf("md5 checksum = %q", got)
	}

	if depth := f.lastRequest(MethodPropfind).Header.Get("Depth"); depth != "0" {
		t.Errorf("Stat used Depth: %q, want 0", depth)
	}
}

// reva returns a second propstat block with status 404 for the properties it
// does not have. It must not overwrite good values.
func TestPropfind404PropstatIsIgnored(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{
		Path:    "/eos/user/e/einstein/notes.txt",
		Size:    42,
		ETag:    "keepme",
		Missing: []string{"oc:checksums", "oc:privatelink"},
	})

	info, err := f.client().Stat(context.Background(), "/eos/user/e/einstein/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.ETag != "keepme" {
		t.Errorf("ETag = %q: the 404 propstat block overwrote the good values", info.ETag)
	}
	if info.Size != 42 {
		t.Errorf("Size = %d, want 42", info.Size)
	}
	if info.Checksums != nil {
		t.Errorf("Checksums = %v, want nil when the server reports them missing", info.Checksums)
	}
}

func TestStatNotFound(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such file", http.StatusNotFound)
	})

	_, err := f.client().Stat(context.Background(), "/eos/user/e/einstein/ghost.txt")
	if cberr.ExitCode(err) != cberr.ExitNotFound {
		t.Errorf("exit code = %d, want %d", cberr.ExitCode(err), cberr.ExitNotFound)
	}
}

func TestListExcludesTheDirectoryItself(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(
		davEntry{Path: "/eos/user/e/einstein/Documents", IsDir: true, Size: 4096},
		davEntry{Path: "/eos/user/e/einstein/Documents/a.txt", Size: 10},
		davEntry{Path: "/eos/user/e/einstein/Documents/sub", IsDir: true, Size: 20},
	)

	entries, err := f.client().List(context.Background(), "/eos/user/e/einstein/Documents")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (the directory itself must be excluded): %+v", len(entries), entries)
	}
	names := []string{entries[0].Name, entries[1].Name}
	if names[0] != "a.txt" || names[1] != "sub" {
		t.Errorf("entries = %v", names)
	}
	if !entries[1].IsDir {
		t.Error("sub should be a directory")
	}
	if depth := f.lastRequest(MethodPropfind).Header.Get("Depth"); depth != "1" {
		t.Errorf("List used Depth: %q, want 1", depth)
	}
}

func TestPropfindRequestsOnlyNeededProperties(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{Path: "/eos/user/e/einstein", IsDir: true})

	if _, err := f.client().Stat(context.Background(), "/eos/user/e/einstein"); err != nil {
		t.Fatal(err)
	}
	body := f.lastRequest(MethodPropfind).Body
	if strings.Contains(body, "allprop") {
		t.Error("PROPFIND should request a named property set, not allprop")
	}
	for _, want := range []string{"oc:fileid", "oc:permissions", "d:getetag", "oc:checksums"} {
		if !strings.Contains(body, want) {
			t.Errorf("PROPFIND body is missing %s:\n%s", want, body)
		}
	}
}

func TestPathEscaping(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{Path: "/eos/user/e/einstein/my notes #1.txt", Size: 1})

	_, err := f.client().Stat(context.Background(), "/eos/user/e/einstein/my notes #1.txt")
	if err != nil {
		t.Fatal(err)
	}
	// httptest decodes the path, so the literal name proves the round trip.
	if got := f.lastRequest(MethodPropfind).Path; !strings.HasSuffix(got, "/my notes #1.txt") {
		t.Errorf("request path = %q, want the escaped name to decode back", got)
	}
}

func TestDavHrefToPath(t *testing.T) {
	tests := []struct {
		href string
		want string
	}{
		{"/remote.php/dav/files/einstein/eos/user/e/einstein/a.txt", "/eos/user/e/einstein/a.txt"},
		{"/remote.php/dav/files/einstein/eos/user/e/einstein/", "/eos/user/e/einstein/"},
		{"/remote.php/dav/files/einstein", "/"},
		{"/remote.php/dav/spaces/localhome$ABC/Documents/a.txt", "/Documents/a.txt"},
		{"/eos/user/e/einstein/a.txt", "/eos/user/e/einstein/a.txt"},
	}
	for _, tt := range tests {
		if got := davHrefToPath(tt.href); got != tt.want {
			t.Errorf("davHrefToPath(%q) = %q, want %q", tt.href, got, tt.want)
		}
	}
}

func TestOcSizePreferredOverContentLength(t *testing.T) {
	// A directory has no d:getcontentlength. oc:size is its recursive size.
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{Path: "/eos/user/e/einstein/Documents", IsDir: true, Size: 987654})

	info, err := f.client().Stat(context.Background(), "/eos/user/e/einstein/Documents")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 987654 {
		t.Errorf("directory Size = %d, want oc:size 987654", info.Size)
	}
}

func TestDownload(t *testing.T) {
	f := newFakeServer(t)
	const content = "hello cernbox"
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, content)
	})

	rc, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/a.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != content {
		t.Errorf("downloaded %q, want %q", got, content)
	}
}

func TestDownloadWithOffsetSendsRange(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, "resumed")
	})

	rc, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/a.txt", 100)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()

	if got := f.lastRequest(http.MethodGet).Header.Get("Range"); got != "bytes=100-" {
		t.Errorf("Range = %q, want bytes=100-", got)
	}
}

func TestDownloadRefusesIgnoredRange(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodGet, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "the whole file from the start")
	})

	_, _, err := f.client().Download(context.Background(), "/eos/user/e/einstein/a.txt", 100)
	if err == nil {
		t.Fatal("expected an error when the server ignores Range")
	}
	if !strings.Contains(err.Error(), "ignored the requested byte range") {
		t.Errorf("error = %v, want it to explain the ignored range", err)
	}
}

func TestMkdir(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodMkcol, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	if err := f.client().Mkdir(context.Background(), "/eos/user/e/einstein/new", false); err != nil {
		t.Fatal(err)
	}
	if n := f.countRequests(MethodMkcol); n != 1 {
		t.Errorf("MKCOL count = %d, want 1", n)
	}
}

// The first segments of a CERNBox path are not real directories, so only the
// missing levels may be created.
func TestMkdirAllCreatesOnlyWhatIsMissing(t *testing.T) {
	f := newFakeServer(t)
	var created []string
	existing := "/eos/user/e/einstein"
	exists := map[string]bool{existing: true}
	f.on(MethodMkcol, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		target := strings.TrimPrefix(r.URL.Path, davFilesPrefix+"/einstein")
		// Like reva, a collection can only be made inside an existing one.
		if !exists[path.Dir(target)] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		exists[target] = true
		created = append(created, target)
		w.WriteHeader(http.StatusCreated)
	})
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	if err := f.client().Mkdir(context.Background(), existing+"/a/b/c", true); err != nil {
		t.Fatal(err)
	}
	want := []string{existing + "/a", existing + "/a/b", existing + "/a/b/c"}
	if len(created) != len(want) {
		t.Fatalf("created %v, want exactly %v", created, want)
	}
	for i, w := range want {
		if created[i] != w {
			t.Errorf("created[%d] = %q, want %q", i, created[i], w)
		}
	}
}

func TestMkdirAllDoesNotTouchTheNamespaceRoot(t *testing.T) {
	f := newFakeServer(t)
	var created []string
	f.on(MethodMkcol, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		created = append(created, r.URL.Path)
		w.WriteHeader(http.StatusCreated)
	})

	if err := f.client().Mkdir(context.Background(), "/eos/user/e/einstein/new", true); err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("created %v, want only the leaf: everything above it already exists", created)
	}
	for _, p := range created {
		if strings.HasSuffix(p, "/eos") {
			t.Errorf("tried to create the namespace root %q", p)
		}
	}
}

// Another client may create the same tree, and a 405 on an existing collection
// is not an error.
func TestMkdirAllToleratesExistingDirectories(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodMkcol, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "already exists", http.StatusMethodNotAllowed)
	})
	f.serveMultistatus(davEntry{Path: "/eos", IsDir: true})

	if err := f.client().Mkdir(context.Background(), "/eos/user", true); err != nil {
		t.Fatalf("Mkdir with existing directories should succeed, got %v", err)
	}
}

func TestMkdirAllPropagatesRealErrors(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodMkcol, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "denied", http.StatusForbidden)
	})

	err := f.client().Mkdir(context.Background(), "/eos/user/e/einstein/x", true)
	if cberr.ExitCode(err) != cberr.ExitPermission {
		t.Errorf("exit code = %d, want %d", cberr.ExitCode(err), cberr.ExitPermission)
	}
}

func TestRemove(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodDelete, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if err := f.client().Remove(context.Background(), "/eos/user/e/einstein/a.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestMoveSetsDestinationAndOverwrite(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodMove, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	c := f.client()
	if err := c.Move(context.Background(), "/eos/user/e/einstein/a.txt", "/eos/user/e/einstein/b.txt", false); err != nil {
		t.Fatal(err)
	}
	req := f.lastRequest(MethodMove)
	if !strings.HasSuffix(req.Header.Get("Destination"), "/eos/user/e/einstein/b.txt") {
		t.Errorf("Destination = %q", req.Header.Get("Destination"))
	}
	if got := req.Header.Get("Overwrite"); got != "F" {
		t.Errorf("Overwrite = %q, want F", got)
	}

	if err := c.Move(context.Background(), "/eos/user/e/einstein/a.txt", "/eos/user/e/einstein/b.txt", true); err != nil {
		t.Fatal(err)
	}
	if got := f.lastRequest(MethodMove).Header.Get("Overwrite"); got != "T" {
		t.Errorf("Overwrite with overwrite=true = %q, want T", got)
	}
}

func TestCopyUsesCopyMethod(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodCopy, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	if err := f.client().Copy(context.Background(), "/eos/a", "/eos/b", true); err != nil {
		t.Fatal(err)
	}
	if n := f.countRequests(MethodCopy); n != 1 {
		t.Errorf("COPY count = %d", n)
	}
}

func TestTouchRefusesToOverwrite(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPut, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	if err := f.client().Touch(context.Background(), "/eos/user/e/einstein/new.txt"); err != nil {
		t.Fatal(err)
	}
	if got := f.lastRequest(http.MethodPut).Header.Get("If-None-Match"); got != "*" {
		t.Errorf("If-None-Match = %q, want *: touch must not truncate an existing file", got)
	}
}

func TestWalk(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		depth := r.Header.Get("Depth")
		switch {
		case strings.HasSuffix(r.URL.Path, "/root") && depth == "0":
			fmt.Fprint(w, multistatusXML("einstein", davEntry{Path: "/root", IsDir: true}))
		case strings.HasSuffix(r.URL.Path, "/root") && depth == "1":
			fmt.Fprint(w, multistatusXML("einstein",
				davEntry{Path: "/root", IsDir: true},
				davEntry{Path: "/root/a.txt", Size: 1},
				davEntry{Path: "/root/sub", IsDir: true},
			))
		case strings.HasSuffix(r.URL.Path, "/root/sub"):
			fmt.Fprint(w, multistatusXML("einstein",
				davEntry{Path: "/root/sub", IsDir: true},
				davEntry{Path: "/root/sub/b.txt", Size: 2},
			))
		default:
			fmt.Fprint(w, multistatusXML("einstein"))
		}
	})

	var seen []string
	err := f.client().Walk(context.Background(), "/root", func(info ResourceInfo) error {
		seen = append(seen, info.Path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"/root", "/root/a.txt", "/root/sub", "/root/sub/b.txt"}
	if len(seen) != len(want) {
		t.Fatalf("walked %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("walk order: got %v, want %v", seen, want)
			break
		}
	}
}

func TestWalkStopsOnCallbackError(t *testing.T) {
	f := newFakeServer(t)
	f.serveMultistatus(davEntry{Path: "/root", IsDir: true})

	sentinel := fmt.Errorf("stop here")
	err := f.client().Walk(context.Background(), "/root", func(ResourceInfo) error {
		return sentinel
	})
	if err != sentinel {
		t.Errorf("Walk returned %v, want the callback's error", err)
	}
}

func TestSearch(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodReport, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, multistatusXML("einstein", davEntry{Path: "/eos/user/e/einstein/notes.txt", Size: 5}))
	})

	results, err := f.client().Search(context.Background(), "/eos/user/e/einstein",
		SearchOptions{Pattern: "notes", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Name != "notes.txt" {
		t.Fatalf("results = %+v", results)
	}

	body := f.lastRequest(MethodReport).Body
	if !strings.Contains(body, "<oc:pattern>notes</oc:pattern>") {
		t.Errorf("REPORT body is missing the pattern:\n%s", body)
	}
	if !strings.Contains(body, "<oc:limit>10</oc:limit>") {
		t.Errorf("REPORT body is missing the limit:\n%s", body)
	}
}

func TestSearchEscapesPattern(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodReport, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, multistatusXML("einstein"))
	})

	_, err := f.client().Search(context.Background(), "/eos", SearchOptions{Pattern: `a<b&c"`})
	if err != nil {
		t.Fatal(err)
	}
	body := f.lastRequest(MethodReport).Body
	if strings.Contains(body, "<oc:pattern>a<b") {
		t.Errorf("pattern was not XML-escaped, the request body is malformed:\n%s", body)
	}
	if !strings.Contains(body, "&lt;") {
		t.Errorf("expected escaped markup in:\n%s", body)
	}
}

func TestParseChecksums(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want map[string]string
	}{
		{"space separated", []string{"SHA1:abc MD5:def"}, map[string]string{"sha1": "abc", "md5": "def"}},
		{"one per element", []string{"SHA1:abc", "MD5:def"}, map[string]string{"sha1": "abc", "md5": "def"}},
		{"empty", []string{""}, nil},
		{"malformed", []string{"garbage"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChecksums(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseChecksums(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("checksum[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"/eos/user//e/./einstein", "/eos/user/e/einstein"},
		{"/eos/user/e/einstein/", "/eos/user/e/einstein/"},
		{"eos/user", "/eos/user"},
	}
	for _, tt := range tests {
		if got := cleanPath(tt.in); got != tt.want {
			t.Errorf("cleanPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMalformedMultistatusIsReported(t *testing.T) {
	f := newFakeServer(t)
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, "<not-xml")
	})

	_, err := f.client().Stat(context.Background(), "/eos/user/e/einstein")
	if err == nil || !strings.Contains(err.Error(), "malformed PROPFIND response") {
		t.Errorf("got %v, want a clear parse error", err)
	}
}

// An empty body is sent as http.NoBody, so the transport never closes the body
// that was opened. An open file cannot be deleted on Windows.
func TestEmptyBodyIsClosed(t *testing.T) {
	f := newFakeServer(t)
	f.on(http.MethodPut, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	body := &closeRecorder{Reader: strings.NewReader("")}
	c := f.client()
	resp, err := c.do(context.Background(), request{
		method: http.MethodPut,
		url:    c.URL(davFilesPrefix + "/einstein/empty.txt"),
		body:   readerBody(func() (io.ReadCloser, error) { return body, nil }, 0),
		op:     "upload",
	})
	if err != nil {
		t.Fatal(err)
	}
	drain(resp)
	if !body.closed {
		t.Error("the body of an empty request was never closed")
	}
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

func TestTouchSendsContentLengthZero(t *testing.T) {
	f := newFakeServer(t)
	var chunked bool
	f.on(http.MethodPut, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		chunked = len(r.TransferEncoding) > 0 && r.TransferEncoding[0] == "chunked"
		w.WriteHeader(http.StatusCreated)
	})

	if err := f.client().Touch(context.Background(), "/eos/user/e/einstein/new.txt"); err != nil {
		t.Fatal(err)
	}
	if chunked {
		t.Error("touch sent a chunked body")
	}
}

func TestPropfindSendsContentLength(t *testing.T) {
	f := newFakeServer(t)
	var gotLength int64 = -1
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		gotLength = r.ContentLength
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, multistatusXML("einstein", davEntry{Path: "/eos/user/e/einstein", IsDir: true}))
	})

	if _, err := f.client().Stat(context.Background(), "/eos/user/e/einstein"); err != nil {
		t.Fatal(err)
	}
	if gotLength != int64(len(propfindBody)) {
		t.Errorf("server saw ContentLength %d, want %d", gotLength, len(propfindBody))
	}
}
