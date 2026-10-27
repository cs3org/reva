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
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	httpbackend "github.com/cs3org/reva/v3/pkg/cli/backend/http"
	"github.com/cs3org/reva/v3/pkg/cli/data"
)

// fakeBox is an in-memory server with WebDAV, TUS, ranged downloads and a tar
// archiver.
type fakeBox struct {
	t  *testing.T
	ts *httptest.Server

	mu      sync.Mutex
	files   map[string][]byte
	dirs    map[string]bool
	mtimes  map[string]time.Time
	uploads map[string]*fakeUpload

	tusEnabled      bool
	archiverEnabled bool
	maxChunk        int64
	// failChunkAfter makes an upload fail once it has this many bytes.
	failChunkAfter int64
	// corruptDownload makes GET return bytes that do not match the checksum.
	corruptDownload bool

	putCount     int
	tusPostCount int
	patchCount   int
	getCount     int
	archiveCount int
}

type fakeUpload struct {
	path   string
	size   int64
	buf    []byte
	failed bool
}

func newFakeBox(t *testing.T) *fakeBox {
	t.Helper()
	b := &fakeBox{
		t:               t,
		files:           map[string][]byte{},
		dirs:            map[string]bool{"/": true},
		mtimes:          map[string]time.Time{},
		uploads:         map[string]*fakeUpload{},
		tusEnabled:      true,
		archiverEnabled: true,
		maxChunk:        1 << 20,
	}
	b.ts = httptest.NewServer(http.HandlerFunc(b.route))
	t.Cleanup(b.ts.Close)
	return b
}

func (b *fakeBox) engine(opts Options) *Engine {
	b.t.Helper()
	if opts.StateDir == "" {
		opts.StateDir = b.t.TempDir()
	}
	hb, err := httpbackend.New(b.ts.URL, httpbackend.Options{
		Credentials: creds{},
		HTTPClient:  b.ts.Client(),
	})
	if err != nil {
		b.t.Fatalf("httpbackend.New: %v", err)
	}
	return New(hb.Files(), data.New(b.ts.Client(), hb.Locator()), opts)
}

type creds struct{}

func (creds) Credential(context.Context) (httpbackend.Credential, error) {
	return httpbackend.Credential{Header: "x-access-token", Value: "test"}, nil
}

const davPrefix = "/remote.php/dav/files/einstein"

func (b *fakeBox) route(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/ocs/"):
		b.serveCapabilities(w)
		return
	case r.URL.Path == "/graph/v1.0/me":
		fmt.Fprint(w, `{"id":"u1","displayName":"Albert Einstein","onPremisesSamAccountName":"einstein"}`)
		return
	case strings.HasPrefix(r.URL.Path, "/archiver"):
		b.serveArchive(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/tus/"):
		b.serveTus(w, r)
		return
	case strings.HasPrefix(r.URL.Path, davPrefix):
		b.serveDav(w, r)
		return
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (b *fakeBox) serveCapabilities(w http.ResponseWriter) {
	tus := ""
	if b.tusEnabled {
		tus = fmt.Sprintf(`"tus_support":{"version":"1.0.0","resumable":"1.0.0","max_chunk_size":%d},`, b.maxChunk)
	}
	archivers := `"archivers":[]`
	if b.archiverEnabled {
		archivers = `"archivers":[{"enabled":true,"version":"2.0.0","formats":["tar","zip"],` +
			`"archiver_url":"/archiver","max_num_files":"10000","max_size":"1073741824"}]`
	}
	fmt.Fprintf(w, `{"ocs":{"data":{"version":{"string":"10.0.11"},"capabilities":{
	  "core":{"webdav-root":"remote.php/webdav"},
	  "checksums":{"supportedTypes":["sha1","md5","adler32"],"preferredUploadType":"md5"},
	  "files":{%s%s}}}}}`, tus, archivers)
}

func (b *fakeBox) serveDav(w http.ResponseWriter, r *http.Request) {
	p := b.davPath(r)

	switch r.Method {
	case "PROPFIND":
		b.servePropfind(w, r, p)
	case http.MethodGet:
		b.serveGet(w, r, p)
	case http.MethodPut:
		b.servePut(w, r, p)
	case "MKCOL":
		b.serveMkcol(w, p)
	case http.MethodPost:
		b.serveTusCreate(w, r, p)
	case http.MethodDelete:
		b.mu.Lock()
		delete(b.files, p)
		delete(b.dirs, p)
		b.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *fakeBox) davPath(r *http.Request) string {
	p := strings.TrimPrefix(r.URL.Path, davPrefix)
	if p == "" {
		p = "/"
	}
	return path.Clean(p)
}

func (b *fakeBox) servePropfind(w http.ResponseWriter, r *http.Request, p string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	depth := r.Header.Get("Depth")
	var entries []davEntry

	switch {
	case b.dirs[p]:
		entries = append(entries, davEntry{Path: p, IsDir: true})
		if depth == "1" {
			entries = append(entries, b.childrenLocked(p)...)
		}
	case b.files[p] != nil:
		entries = append(entries, davEntry{Path: p, Size: int64(len(b.files[p])), Body: b.files[p], Modified: b.mtimes[p]})
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	fmt.Fprint(w, multistatus(entries))
}

func (b *fakeBox) childrenLocked(dir string) []davEntry {
	var out []davEntry
	seen := map[string]bool{}
	for p, body := range b.files {
		if path.Dir(p) == dir {
			out = append(out, davEntry{Path: p, Size: int64(len(body)), Body: body, Modified: b.mtimes[p]})
		}
	}
	for p := range b.dirs {
		if p != dir && path.Dir(p) == dir && !seen[p] {
			out = append(out, davEntry{Path: p, IsDir: true})
			seen[p] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (b *fakeBox) serveGet(w http.ResponseWriter, r *http.Request, p string) {
	b.mu.Lock()
	body, ok := b.files[p]
	b.corruptIfAskedLocked(&body)
	b.getCount++
	b.mu.Unlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if rangeHdr := r.Header.Get("Range"); rangeHdr != "" {
		var start int64
		fmt.Sscanf(rangeHdr, "bytes=%d-", &start)
		if start > int64(len(body)) {
			start = int64(len(body))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)-int(start)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body[start:])
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Write(body)
}

func (b *fakeBox) corruptIfAskedLocked(body *[]byte) {
	if b.corruptDownload && len(*body) > 0 {
		corrupted := make([]byte, len(*body))
		copy(corrupted, *body)
		corrupted[0] ^= 0xff
		*body = corrupted
	}
}

func (b *fakeBox) servePut(w http.ResponseWriter, r *http.Request, p string) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	b.files[p] = body
	b.mtimes[p] = time.Now()
	b.putCount++
	b.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
}

func (b *fakeBox) serveMkcol(w http.ResponseWriter, p string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dirs[p] {
		http.Error(w, "already exists", http.StatusMethodNotAllowed)
		return
	}
	b.dirs[p] = true
	w.WriteHeader(http.StatusCreated)
}

// serveTusCreate handles the POST of TUS. It addresses the parent directory and
// names the file in Upload-Metadata.
func (b *fakeBox) serveTusCreate(w http.ResponseWriter, r *http.Request, dir string) {
	meta := parseTusMetadata(r.Header.Get("Upload-Metadata"))
	name := meta["filename"]
	if name == "" {
		http.Error(w, "no filename", http.StatusPreconditionFailed)
		return
	}
	size, _ := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)

	b.mu.Lock()
	id := fmt.Sprintf("upload-%d", len(b.uploads)+1)
	b.uploads[id] = &fakeUpload{path: path.Join(dir, name), size: size}
	b.tusPostCount++
	b.mu.Unlock()

	w.Header().Set("Location", b.ts.URL+"/tus/"+id)
	w.Header().Set("Tus-Resumable", "1.0.0")
	w.WriteHeader(http.StatusCreated)
}

func (b *fakeBox) serveTus(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/tus/")

	b.mu.Lock()
	up, ok := b.uploads[id]
	b.mu.Unlock()
	if !ok {
		http.Error(w, "no such upload", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodHead:
		b.mu.Lock()
		offset := len(up.buf)
		b.mu.Unlock()
		w.Header().Set("Upload-Offset", strconv.Itoa(offset))
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.WriteHeader(http.StatusOK)

	case http.MethodPatch:
		chunk, _ := io.ReadAll(r.Body)
		b.mu.Lock()
		b.patchCount++
		if b.failChunkAfter > 0 && int64(len(up.buf))+int64(len(chunk)) > b.failChunkAfter && !up.failed {
			accept := b.failChunkAfter - int64(len(up.buf))
			if accept > 0 {
				up.buf = append(up.buf, chunk[:accept]...)
			}
			up.failed = true
			b.mu.Unlock()
			http.Error(w, "storage hiccup", http.StatusServiceUnavailable)
			return
		}
		up.buf = append(up.buf, chunk...)
		offset := len(up.buf)
		if int64(offset) >= up.size {
			b.files[up.path] = up.buf
			b.mtimes[up.path] = time.Now()
		}
		b.mu.Unlock()

		w.Header().Set("Upload-Offset", strconv.Itoa(offset))
		w.Header().Set("Tus-Resumable", "1.0.0")
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (b *fakeBox) serveArchive(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	b.archiveCount++
	roots := r.URL.Query()["path"]
	files := map[string][]byte{}
	dirs := []string{}
	for _, root := range roots {
		for p, body := range b.files {
			if p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
				files[p] = body
			}
		}
		for p := range b.dirs {
			if strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/") {
				dirs = append(dirs, p)
			}
		}
	}
	base := ""
	if len(roots) == 1 {
		base = path.Dir(strings.TrimSuffix(roots[0], "/"))
	}
	b.mu.Unlock()

	w.Header().Set("Content-Type", "application/x-tar")
	tw := tar.NewWriter(w)
	sort.Strings(dirs)
	for _, d := range dirs {
		tw.WriteHeader(&tar.Header{Name: relTo(base, d), Typeflag: tar.TypeDir, Mode: 0o755})
	}
	var names []string
	for p := range files {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		body := files[p]
		tw.WriteHeader(&tar.Header{Name: relTo(base, p), Size: int64(len(body)), Mode: 0o644})
		tw.Write(body)
	}
	tw.Close()
}

func relTo(base, p string) string {
	rel := strings.TrimPrefix(p, strings.TrimSuffix(base, "/")+"/")
	return strings.TrimPrefix(rel, "/")
}

type davEntry struct {
	Path     string
	IsDir    bool
	Size     int64
	Body     []byte
	Modified time.Time
}

func multistatus(entries []davEntry) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">`)
	for _, e := range entries {
		href := davPrefix + e.Path
		if e.IsDir {
			href += "/"
		}
		sb.WriteString("<d:response><d:href>" + xmlEscape(href) + "</d:href>")
		sb.WriteString(`<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>`)
		sb.WriteString("<d:displayname>" + xmlEscape(path.Base(e.Path)) + "</d:displayname>")
		if e.IsDir {
			sb.WriteString("<d:resourcetype><d:collection/></d:resourcetype>")
		} else {
			sb.WriteString("<d:resourcetype></d:resourcetype>")
			fmt.Fprintf(&sb, "<d:getcontentlength>%d</d:getcontentlength>", e.Size)
			if e.Body != nil {
				sb.WriteString("<oc:checksums><oc:checksum>MD5:" + md5hex(e.Body) + "</oc:checksum></oc:checksums>")
			}
		}
		fmt.Fprintf(&sb, "<oc:size>%d</oc:size>", e.Size)
		if !e.Modified.IsZero() {
			sb.WriteString("<d:getlastmodified>" + e.Modified.UTC().Format(http.TimeFormat) + "</d:getlastmodified>")
		}
		sb.WriteString("<oc:fileid>localhome$ABC!" + xmlEscape(e.Path) + "</oc:fileid>")
		sb.WriteString("</d:prop></d:propstat></d:response>")
	}
	sb.WriteString("</d:multistatus>")
	return sb.String()
}

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func parseTusMetadata(header string) map[string]string {
	out := map[string]string{}
	for pair := range strings.SplitSeq(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(pair), " ")
		if !ok {
			out[key] = ""
			continue
		}
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			out[key] = string(decoded)
		}
	}
	return out
}

func (b *fakeBox) putFile(p string, body []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.files[p] = body
	b.mtimes[p] = time.Now()
	for dir := path.Dir(p); dir != "/" && dir != "."; dir = path.Dir(dir) {
		b.dirs[dir] = true
	}
}

func (b *fakeBox) mkdir(p string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for dir := p; dir != "/" && dir != "."; dir = path.Dir(dir) {
		b.dirs[dir] = true
	}
}

func (b *fakeBox) fileContent(p string) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	body, ok := b.files[p]
	return body, ok
}

func (b *fakeBox) hasDir(p string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dirs[p]
}

func (b *fakeBox) requireContent(p string, want []byte) {
	b.t.Helper()
	got, ok := b.fileContent(p)
	if !ok {
		b.t.Fatalf("%s was not uploaded", p)
	}
	if !bytes.Equal(got, want) {
		b.t.Fatalf("%s: uploaded %d bytes, want %d with other content", p, len(got), len(want))
	}
}

func md5hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}
