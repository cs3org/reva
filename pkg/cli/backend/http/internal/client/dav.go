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
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// WebDAV methods that net/http does not define.
const (
	MethodPropfind = "PROPFIND"
	MethodMkcol    = "MKCOL"
	MethodMove     = "MOVE"
	MethodCopy     = "COPY"
	MethodReport   = "REPORT"
)

// ResourceInfo describes one file or directory.
type ResourceInfo struct {
	// Path is the absolute CERNBox path.
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	ETag     string    `json:"etag,omitempty"`
	// ID is the resource id. It stays the same across renames.
	ID          string            `json:"id,omitempty"`
	MimeType    string            `json:"mime_type,omitempty"`
	Permissions string            `json:"permissions,omitempty"`
	Checksums   map[string]string `json:"checksums,omitempty"`
	WebURL      string            `json:"web_url,omitempty"`
}

type multistatus struct {
	XMLName   xml.Name           `xml:"DAV: multistatus"`
	Responses []multistatusEntry `xml:"DAV: response"`
}

type multistatusEntry struct {
	Href     string     `xml:"DAV: href"`
	Propstat []propstat `xml:"DAV: propstat"`
}

type propstat struct {
	Status string   `xml:"DAV: status"`
	Prop   propsRaw `xml:"DAV: prop"`
}

type propsRaw struct {
	DisplayName  string        `xml:"DAV: displayname"`
	ResourceType *resourceType `xml:"DAV: resourcetype"`
	GetLastMod   string        `xml:"DAV: getlastmodified"`
	GetLength    string        `xml:"DAV: getcontentlength"`
	GetType      string        `xml:"DAV: getcontenttype"`
	GetETag      string        `xml:"DAV: getetag"`
	OCID         string        `xml:"http://owncloud.org/ns fileid"`
	OCPerms      string        `xml:"http://owncloud.org/ns permissions"`
	OCSize       string        `xml:"http://owncloud.org/ns size"`
	OCLink       string        `xml:"http://owncloud.org/ns privatelink"`
	OCChecksums  *checksumsRaw `xml:"http://owncloud.org/ns checksums"`

	TrashFilename  string `xml:"http://owncloud.org/ns trashbin-original-filename"`
	TrashLocation  string `xml:"http://owncloud.org/ns trashbin-original-location"`
	TrashTimestamp string `xml:"http://owncloud.org/ns trashbin-delete-timestamp"`
	TrashDatetime  string `xml:"http://owncloud.org/ns trashbin-delete-datetime"`
}

type resourceType struct {
	Collection *struct{} `xml:"DAV: collection"`
}

type checksumsRaw struct {
	Checksum []string `xml:"http://owncloud.org/ns checksum"`
}

// propfindBody asks for a named set of properties instead of allprop, to keep
// the response of a large directory small.
const propfindBody = `<?xml version="1.0" encoding="UTF-8"?>
<d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <d:getlastmodified/>
    <d:getcontentlength/>
    <d:getcontenttype/>
    <d:getetag/>
    <oc:fileid/>
    <oc:permissions/>
    <oc:size/>
    <oc:privatelink/>
    <oc:checksums/>
  </d:prop>
</d:propfind>`

// davURL builds the WebDAV URL of an absolute CERNBox path. At CERN,
// /remote.php/dav/files/{user} accepts absolute CS3 paths.
func (c *Client) davURL(ctx context.Context, p string) (string, error) {
	me, err := c.Me(ctx)
	if err != nil {
		return "", err
	}
	return c.URL(davFilesPrefix + "/" + escapePath(me.Username) + escapePath(p)), nil
}

// escapePath percent-encodes each segment of a path. It adds no leading slash,
// because callers join the result onto a prefix.
func escapePath(p string) string {
	if p == "" {
		return ""
	}
	leading := strings.HasPrefix(p, "/")
	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	out := strings.Join(segs, "/")
	if leading {
		out = "/" + out
	}
	return out
}

// Stat returns metadata for a single resource.
func (c *Client) Stat(ctx context.Context, p string) (*ResourceInfo, error) {
	infos, err := c.propfind(ctx, p, 0, "stat")
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, cberr.New(cberr.KindNotFound, "stat", p, "no such file or directory")
	}
	return &infos[0], nil
}

// List returns the direct children of a directory.
func (c *Client) List(ctx context.Context, p string) ([]ResourceInfo, error) {
	infos, err := c.propfind(ctx, p, 1, "list")
	if err != nil {
		return nil, err
	}
	self := strings.TrimSuffix(cleanPath(p), "/")
	out := make([]ResourceInfo, 0, len(infos))
	for _, info := range infos {
		if strings.TrimSuffix(info.Path, "/") == self {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func (c *Client) propfind(ctx context.Context, p string, depth int, op string) ([]ResourceInfo, error) {
	u, err := c.davURL(ctx, p)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, request{
		method: MethodPropfind,
		url:    u,
		header: http.Header{
			"Depth":        []string{strconv.Itoa(depth)},
			"Content-Type": []string{"application/xml"},
		},
		body:    stringBody(propfindBody),
		op:      op,
		path:    p,
		expects: []int{http.StatusMultiStatus, http.StatusOK},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return parseMultistatus(resp.Body, op, p)
}

func decodeXML(r io.Reader, v any, op, p string) error {
	if err := xml.NewDecoder(r).Decode(v); err != nil {
		return cberr.Wrap(cberr.KindOther, op, p, fmt.Errorf("malformed PROPFIND response: %w", err))
	}
	return nil
}

func parseMultistatus(r io.Reader, op, p string) ([]ResourceInfo, error) {
	var ms multistatus
	if err := decodeXML(r, &ms, op, p); err != nil {
		return nil, err
	}

	out := make([]ResourceInfo, 0, len(ms.Responses))
	for _, entry := range ms.Responses {
		info, ok := entryToInfo(entry)
		if !ok {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func entryToInfo(entry multistatusEntry) (ResourceInfo, bool) {
	href, err := url.PathUnescape(entry.Href)
	if err != nil {
		href = entry.Href
	}

	// The 404 propstat block lists properties the server does not have.
	props, found := okProps(entry)
	if !found {
		return ResourceInfo{}, false
	}

	// Collection hrefs end in a slash. Drop it, so that List and Walk compare
	// one spelling.
	p := strings.TrimSuffix(davHrefToPath(href), "/")
	if p == "" {
		p = "/"
	}
	info := ResourceInfo{
		Path:        p,
		Name:        props.DisplayName,
		IsDir:       props.ResourceType != nil && props.ResourceType.Collection != nil,
		ETag:        strings.Trim(props.GetETag, `"`),
		ID:          props.OCID,
		MimeType:    props.GetType,
		Permissions: props.OCPerms,
		WebURL:      props.OCLink,
	}
	if info.Name == "" {
		info.Name = path.Base(p)
	}
	if t, err := http.ParseTime(props.GetLastMod); err == nil {
		info.Modified = t
	}
	// oc:size is the recursive size of a collection. getcontentlength is absent
	// on collections.
	if n, err := strconv.ParseInt(props.OCSize, 10, 64); err == nil {
		info.Size = n
	} else if n, err := strconv.ParseInt(props.GetLength, 10, 64); err == nil {
		info.Size = n
	}
	if props.OCChecksums != nil {
		info.Checksums = parseChecksums(props.OCChecksums.Checksum)
	}
	return info, true
}

// parseChecksums reads "SHA1:abc MD5:def". reva sends them space-separated in
// one element, or one per element.
func parseChecksums(raw []string) map[string]string {
	out := map[string]string{}
	for _, chunk := range raw {
		for field := range strings.FieldsSeq(chunk) {
			name, value, ok := strings.Cut(field, ":")
			if !ok || value == "" {
				continue
			}
			out[strings.ToLower(name)] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// davHrefToPath turns an href such as /remote.php/dav/files/{user}/eos/x into
// the absolute CERNBox path.
func davHrefToPath(href string) string {
	for _, prefix := range []string{davFilesPrefix, davSpacesPrefix} {
		if _, after, ok := strings.Cut(href, prefix); ok {
			// Drop the {user} or {space} segment.
			if _, after, ok := strings.Cut(strings.TrimPrefix(after, "/"), "/"); ok {
				return cleanPath("/" + after)
			}
			return "/"
		}
	}
	return cleanPath(href)
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	trailing := strings.HasSuffix(p, "/") && len(p) > 1
	cleaned := path.Clean("/" + strings.TrimPrefix(p, "/"))
	if trailing && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

// Download opens a file for reading, starting at offset. The caller must close
// the returned ReadCloser.
func (c *Client) Download(ctx context.Context, p string, offset int64) (io.ReadCloser, int64, error) {
	u, err := c.davURL(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	header := http.Header{}
	expects := []int{http.StatusOK}
	if offset > 0 {
		header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		expects = append(expects, http.StatusPartialContent)
	}

	resp, err := c.do(ctx, request{
		method:  http.MethodGet,
		url:     u,
		header:  header,
		op:      "download",
		path:    p,
		expects: expects,
	})
	if err != nil {
		return nil, 0, err
	}
	// A 200 means the server ignored Range. Refuse, or the caller gets a
	// duplicated prefix.
	if offset > 0 && resp.StatusCode == http.StatusOK {
		drain(resp)
		return nil, 0, cberr.New(cberr.KindOther, "resume download", p,
			"the server ignored the requested byte range")
	}
	if offset > 0 {
		body, err := guardChunkedRange(resp, p)
		if err != nil {
			drain(resp)
			return nil, 0, err
		}
		return body, resp.ContentLength, nil
	}
	return resp.Body, resp.ContentLength, nil
}

// guardChunkedRange refuses a ranged response that has chunked framing inside
// its body.
//
// EOS does this: bytes=136- of a 191-byte file came back 206 with
// Content-Length 55 and a body that began "37\r\n" (0x37 is 55). The framing
// counts in the length, so four bytes of the file are lost at the end. reva
// forwards it as it is. The only sign is a body that starts with its own length
// in hex and CRLF. Without this check, a resumed download has the right size
// and the wrong bytes.
func guardChunkedRange(resp *http.Response, p string) (io.ReadCloser, error) {
	if resp.ContentLength <= 0 {
		return resp.Body, nil
	}
	marker := fmt.Sprintf("%x\r\n", resp.ContentLength)

	// Read only the marker length, and put it back in front of the body.
	head := make([]byte, len(marker))
	n, err := io.ReadFull(resp.Body, head)
	head = head[:n]
	if string(head) == marker {
		return nil, cberr.New(cberr.KindOther, "read a byte range of", p,
			"the storage answered with chunked framing inside a body it gave a "+
				"length, so the bytes are not the file's. Resuming a download and "+
				"following a file both need working byte ranges; this needs fixing "+
				"in the storage, not here")
	}
	body := io.NopCloser(io.MultiReader(bytes.NewReader(head), resp.Body))
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, cberr.Wrap(cberr.KindOther, "download", p, err)
	}
	return &joinedBody{Reader: body, closer: resp.Body}, nil
}

type joinedBody struct {
	io.Reader
	closer io.Closer
}

func (b *joinedBody) Close() error { return b.closer.Close() }

// Mkdir creates a directory. With parents, missing ancestors are created too.
func (c *Client) Mkdir(ctx context.Context, p string, parents bool) error {
	if parents {
		return c.mkdirAll(ctx, p)
	}
	return c.mkcol(ctx, p)
}

func (c *Client) mkcol(ctx context.Context, p string) error {
	u, err := c.davURL(ctx, p)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, request{
		method:  MethodMkcol,
		url:     u,
		op:      "create directory",
		path:    p,
		expects: []int{http.StatusCreated, http.StatusOK},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// mkdirAll creates a directory and any missing ancestor.
//
// It works down from the target. Creating every prefix from the root would fail
// at once, because the first segments are not real directories: "/eos" cannot
// be created or stat'ed. An existing directory at any level is not an error, so
// this is safe next to a client that creates the same tree.
func (c *Client) mkdirAll(ctx context.Context, p string) error {
	p = cleanPath(p)
	if p == "/" {
		return nil
	}

	err := c.mkcol(ctx, p)
	if err == nil {
		return nil
	}
	if c.isExistingDir(ctx, p) {
		return nil
	}

	// The parent may be missing. Create it and try again.
	parent := cleanPath(path.Dir(p))
	if parent == p || parent == "/" {
		return err
	}
	if parentErr := c.mkdirAll(ctx, parent); parentErr != nil {
		return parentErr
	}
	if err := c.mkcol(ctx, p); err != nil {
		if c.isExistingDir(ctx, p) {
			return nil
		}
		return err
	}
	return nil
}

func (c *Client) isExistingDir(ctx context.Context, p string) bool {
	info, err := c.Stat(ctx, p)
	return err == nil && info.IsDir
}

// Remove deletes a file or directory. DELETE on a collection is recursive, so
// the caller must refuse a non-empty directory without -r.
func (c *Client) Remove(ctx context.Context, p string) error {
	u, err := c.davURL(ctx, p)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, request{
		method:  http.MethodDelete,
		url:     u,
		op:      "remove",
		path:    p,
		expects: []int{http.StatusNoContent, http.StatusOK, http.StatusAccepted},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// Move renames or moves a resource server-side.
func (c *Client) Move(ctx context.Context, src, dst string, overwrite bool) error {
	return c.moveOrCopy(ctx, MethodMove, "move", src, dst, overwrite)
}

// Copy duplicates a resource on the server.
func (c *Client) Copy(ctx context.Context, src, dst string, overwrite bool) error {
	return c.moveOrCopy(ctx, MethodCopy, "copy", src, dst, overwrite)
}

func (c *Client) moveOrCopy(ctx context.Context, method, op, src, dst string, overwrite bool) error {
	srcURL, err := c.davURL(ctx, src)
	if err != nil {
		return err
	}
	dstURL, err := c.davURL(ctx, dst)
	if err != nil {
		return err
	}
	header := http.Header{"Destination": []string{dstURL}}
	if overwrite {
		header.Set("Overwrite", "T")
	} else {
		header.Set("Overwrite", "F")
	}

	resp, err := c.do(ctx, request{
		method:  method,
		url:     srcURL,
		header:  header,
		op:      op,
		path:    src,
		expects: []int{http.StatusCreated, http.StatusNoContent, http.StatusOK},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// Touch creates an empty file, failing if it already exists.
func (c *Client) Touch(ctx context.Context, p string) error {
	// reva ignores If-None-Match: * and truncates an existing file, so look
	// first.
	if info, statErr := c.Stat(ctx, p); statErr == nil {
		if info.IsDir {
			return cberr.New(cberr.KindConflict, "create file", p, "is a directory")
		}
		return cberr.New(cberr.KindConflict, "create file", p, "already exists")
	}

	u, err := c.davURL(ctx, p)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, request{
		method:  http.MethodPut,
		url:     u,
		header:  http.Header{"If-None-Match": []string{"*"}},
		body:    stringBody(""),
		op:      "create file",
		path:    p,
		expects: []int{http.StatusCreated, http.StatusOK, http.StatusNoContent},
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// Walk calls fn for p and everything beneath it, depth first. A directory comes
// before its children.
func (c *Client) Walk(ctx context.Context, p string, fn func(ResourceInfo) error) error {
	info, err := c.Stat(ctx, p)
	if err != nil {
		return err
	}
	return c.walk(ctx, *info, fn)
}

func (c *Client) walk(ctx context.Context, info ResourceInfo, fn func(ResourceInfo) error) error {
	if err := fn(info); err != nil {
		return err
	}
	if !info.IsDir {
		return nil
	}
	children, err := c.List(ctx, info.Path)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.walk(ctx, child, fn); err != nil {
			return err
		}
	}
	return nil
}

// SearchOptions constrains a REPORT search.
type SearchOptions struct {
	Pattern string
	Limit   int
}

const searchReportBody = `<?xml version="1.0" encoding="UTF-8"?>
<oc:search-files xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns">
  <d:prop>
    <d:displayname/>
    <d:resourcetype/>
    <d:getlastmodified/>
    <d:getcontentlength/>
    <d:getcontenttype/>
    <d:getetag/>
    <oc:fileid/>
    <oc:permissions/>
    <oc:size/>
  </d:prop>
  <oc:search>
    <oc:pattern>%s</oc:pattern>
    <oc:limit>%d</oc:limit>
  </oc:search>
</oc:search-files>`

// ErrSearchUnsupported means the server cannot search. The caller should walk
// the tree instead.
var ErrSearchUnsupported = errors.New("the server does not support search")

// Search runs a server-side search under p. reva's search-files handler answers
// 501, so this returns ErrSearchUnsupported there.
func (c *Client) Search(ctx context.Context, p string, opts SearchOptions) ([]ResourceInfo, error) {
	u, err := c.davURL(ctx, p)
	if err != nil {
		return nil, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}
	body := fmt.Sprintf(searchReportBody, xmlEscape(opts.Pattern), limit)

	resp, err := c.do(ctx, request{
		method:  MethodReport,
		url:     u,
		header:  http.Header{"Content-Type": []string{"application/xml"}},
		body:    stringBody(body),
		op:      "search",
		path:    p,
		expects: []int{http.StatusMultiStatus, http.StatusOK},
	})
	if err != nil {
		var ce *cberr.Error
		if errors.As(err, &ce) && ce.Status == http.StatusNotImplemented {
			return nil, ErrSearchUnsupported
		}
		return nil, err
	}
	defer resp.Body.Close()

	return parseMultistatus(resp.Body, "search", p)
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
