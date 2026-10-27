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
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// fakeServer is a stand-in CERNBox. A test can override any route.
type fakeServer struct {
	t  *testing.T
	ts *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest

	// Handlers, keyed by "METHOD /path-prefix".
	overrides map[string]http.HandlerFunc

	capabilitiesJSON string
	meJSON           string
	drivesJSON       string
}

type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   string
}

const defaultCapabilities = `{
  "ocs": {
    "meta": {"status": "ok", "statuscode": 100},
    "data": {
      "version": {"string": "10.0.11", "edition": "reva"},
      "capabilities": {
        "core": {"webdav-root": "remote.php/webdav", "status": {"productname": "reva", "versionstring": "10.0.11"}},
        "checksums": {"supportedTypes": ["sha1", "md5", "adler32"], "preferredUploadType": "md5"},
        "files": {
          "undelete": "1",
          "versioning": true,
          "tus_support": {"version": "1.0.0", "resumable": "1.0.0", "max_chunk_size": 1048576},
          "archivers": [
            {"enabled": true, "version": "2.0.0", "formats": ["tar", "zip"],
             "archiver_url": "/archiver", "max_num_files": "10000", "max_size": "1073741824"}
          ]
        },
        "files_sharing": {"api_enabled": 1, "public": {"enabled": true}},
        "spaces": {"enabled": true, "projects": true}
      }
    }
  }
}`

const defaultMe = `{
  "id": "4c510ada-c86b-4815-8820-42cdf82c3d51",
  "displayName": "Albert Einstein",
  "mail": "einstein@cern.ch",
  "onPremisesSamAccountName": "einstein"
}`

const defaultDrives = `{
  "value": [
    {
      "id": "localhome$MFZWI...",
      "name": "einstein",
      "driveType": "personal",
      "driveAlias": "eos/user/e/einstein",
      "root": {"webDavUrl": "http://cernbox.test/remote.php/dav/spaces/localhome$MFZWI.../"},
      "quota": {"total": 1073741824, "used": 524288, "remaining": 1073217536}
    },
    {
      "id": "localhome$OJSWI...",
      "name": "cernbox",
      "driveType": "project",
      "driveAlias": "eos/project/c/cernbox",
      "root": {"webDavUrl": "http://cernbox.test/remote.php/dav/spaces/localhome$OJSWI.../"}
    }
  ]
}`

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:                t,
		overrides:        map[string]http.HandlerFunc{},
		capabilitiesJSON: defaultCapabilities,
		meJSON:           defaultMe,
		drivesJSON:       defaultDrives,
	}
	f.ts = httptest.NewServer(http.HandlerFunc(f.route))
	t.Cleanup(f.ts.Close)
	return f
}

func (f *fakeServer) on(method, prefix string, h http.HandlerFunc) {
	f.overrides[method+" "+prefix] = h
}

func (f *fakeServer) route(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{
		Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body),
	})
	f.mu.Unlock()

	for key, h := range f.overrides {
		method, prefix, _ := strings.Cut(key, " ")
		if r.Method == method && strings.HasPrefix(r.URL.Path, prefix) {
			h(w, r)
			return
		}
	}

	switch {
	case strings.HasPrefix(r.URL.Path, ocsCapabilities):
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, f.capabilitiesJSON)
	case r.URL.Path == graphV1+"/me":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, f.meJSON)
	case r.URL.Path == graphBeta+"/me/drives":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, f.drivesJSON)
	default:
		http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
	}
}

func (f *fakeServer) client(opts ...Option) *Client {
	f.t.Helper()
	base := append([]Option{
		WithCredentials(CredentialFunc(func(context.Context) (Credential, error) {
			return Credential{Header: "Authorization", Value: "Bearer test-token"}, nil
		})),
		WithMaxRetries(0),
	}, opts...)
	c, err := New(f.ts.URL, base...)
	if err != nil {
		f.t.Fatalf("New: %v", err)
	}
	return c
}

func (f *fakeServer) recorded() []recordedRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

func (f *fakeServer) lastRequest(method string) recordedRequest {
	f.t.Helper()
	reqs := f.recorded()
	for _, req := range slices.Backward(reqs) {
		if req.Method == method {
			return req
		}
	}
	f.t.Fatalf("no %s request was made; saw %v", method, methodsOf(reqs))
	return recordedRequest{}
}

func (f *fakeServer) countRequests(method string) int {
	n := 0
	for _, r := range f.recorded() {
		if r.Method == method {
			n++
		}
	}
	return n
}

func methodsOf(reqs []recordedRequest) []string {
	out := make([]string, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

// davEntry describes one resource for the multistatus builder.
type davEntry struct {
	Path      string
	IsDir     bool
	Size      int64
	ETag      string
	FileID    string
	Modified  string
	MimeType  string
	Perms     string
	Checksums string
	// Missing lists properties that the server reports as 404.
	Missing []string
}

// multistatusXML renders a PROPFIND response with separate 200 and 404
// propstat blocks, like reva.
func multistatusXML(user string, entries ...davEntry) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	sb.WriteString(`<d:multistatus xmlns:d="DAV:" xmlns:s="http://sabredav.org/ns" xmlns:oc="http://owncloud.org/ns">`)
	for _, e := range entries {
		href := davFilesPrefix + "/" + user + e.Path
		if e.IsDir && !strings.HasSuffix(href, "/") {
			href += "/"
		}
		sb.WriteString("<d:response><d:href>" + xmlEscape(href) + "</d:href>")
		sb.WriteString("<d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop>")
		sb.WriteString("<d:displayname>" + xmlEscape(baseName(e.Path)) + "</d:displayname>")
		if e.IsDir {
			sb.WriteString("<d:resourcetype><d:collection/></d:resourcetype>")
		} else {
			sb.WriteString("<d:resourcetype></d:resourcetype>")
			fmt.Fprintf(&sb, "<d:getcontentlength>%d</d:getcontentlength>", e.Size)
		}
		fmt.Fprintf(&sb, "<oc:size>%d</oc:size>", e.Size)
		if e.Modified != "" {
			sb.WriteString("<d:getlastmodified>" + e.Modified + "</d:getlastmodified>")
		}
		if e.ETag != "" {
			sb.WriteString(`<d:getetag>&quot;` + e.ETag + `&quot;</d:getetag>`)
		}
		if e.FileID != "" {
			sb.WriteString("<oc:fileid>" + xmlEscape(e.FileID) + "</oc:fileid>")
		}
		if e.MimeType != "" {
			sb.WriteString("<d:getcontenttype>" + e.MimeType + "</d:getcontenttype>")
		}
		if e.Perms != "" {
			sb.WriteString("<oc:permissions>" + e.Perms + "</oc:permissions>")
		}
		if e.Checksums != "" {
			sb.WriteString("<oc:checksums><oc:checksum>" + e.Checksums + "</oc:checksum></oc:checksums>")
		}
		sb.WriteString("</d:prop></d:propstat>")

		if len(e.Missing) > 0 {
			sb.WriteString("<d:propstat><d:status>HTTP/1.1 404 Not Found</d:status><d:prop>")
			for _, m := range e.Missing {
				sb.WriteString("<" + m + "/>")
			}
			sb.WriteString("</d:prop></d:propstat>")
		}
		sb.WriteString("</d:response>")
	}
	sb.WriteString("</d:multistatus>")
	return sb.String()
}

func baseName(p string) string {
	p = strings.TrimSuffix(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func (f *fakeServer) serveMultistatus(entries ...davEntry) {
	f.on(MethodPropfind, davFilesPrefix, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		fmt.Fprint(w, multistatusXML("einstein", entries...))
	})
}

func openerOf(s string) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(s)), nil }
}
