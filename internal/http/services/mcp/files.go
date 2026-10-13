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

package mcp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/datagateway"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/spaces"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// entry is a file or folder as every tool returns it.
type entry struct {
	Path     string `json:"path"`
	ID       string `json:"id"`
	Type     string `json:"type" jsonschema:"file or folder"`
	Size     uint64 `json:"size"`
	Mtime    string `json:"mtime" jsonschema:"last modification time, RFC 3339"`
	MimeType string `json:"mimetype"`
	Etag     string `json:"etag" jsonschema:"changes with every modification; pass it as if_match to write_file and delete"`
	WebURL   string `json:"web_url" jsonschema:"link to open it in CERNBox, to cite to the user"`
}

func (s *svc) entry(info *provider.ResourceInfo) entry {
	return entry{
		Path:     info.GetPath(),
		ID:       resourceID(info.GetId()),
		Type:     resourceType(info.GetType()),
		Size:     info.GetSize(),
		Mtime:    formatTime(info.GetMtime()),
		MimeType: info.GetMimeType(),
		Etag:     info.GetEtag(),
		WebURL:   s.webURL(info.GetPath(), info.GetId()),
	}
}

func resourceID(id *provider.ResourceId) string {
	if id == nil {
		return ""
	}
	return spaces.EncodeToStringifiedResourceID(id)
}

func resourceType(t provider.ResourceType) string {
	switch t {
	case provider.ResourceType_RESOURCE_TYPE_CONTAINER:
		return "folder"
	case provider.ResourceType_RESOURCE_TYPE_FILE:
		return "file"
	case provider.ResourceType_RESOURCE_TYPE_SYMLINK:
		return "symlink"
	case provider.ResourceType_RESOURCE_TYPE_REFERENCE:
		return "reference"
	}
	return "unknown"
}

func formatTime(ts *types.Timestamp) string {
	if ts == nil {
		return ""
	}
	return time.Unix(int64(ts.GetSeconds()), int64(ts.GetNanos())).UTC().Format(time.RFC3339)
}

// webURL is where the user opens a path in the web UI.
func (s *svc) webURL(p string, id *provider.ResourceId) string {
	if s.c.WebBaseURL == "" || p == "" {
		return ""
	}
	segments := strings.Split(p, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.NewReplacer(
		"{base}", s.c.WebBaseURL,
		"{path}", strings.Join(segments, "/"),
		"{id}", url.PathEscape(resourceID(id)),
	).Replace(s.c.WebURLTemplate)
}

func (c *call) stat(p string) (*provider.ResourceInfo, error) {
	res, err := c.gw.Stat(c.ctx, &provider.StatRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return nil, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return nil, err
	}
	return res.GetInfo(), nil
}

func sameEtag(a, b string) bool {
	return strings.Trim(a, `"`) == strings.Trim(b, `"`)
}

func (s *svc) registerFileTools() {
	addTool(s, toolSpec{
		name: toolListSpaces,
		description: "List the storage spaces of the user: their home folder and the project spaces they can access, with the absolute path of each. " +
			"Call it first to know where to start browsing. When the connection is restricted to some folders, roots lists them and only paths under them are accessible.",
		readOnly: true,
	}, s.listSpaces)

	addTool(s, toolSpec{
		name: toolListFolder,
		description: "List the files and folders in a folder, sorted by path, a page at a time. " +
			"path is absolute, such as /eos/user/j/jdoe/docs. When next_cursor is set, call again with it as cursor for the next page.",
		readOnly: true,
	}, s.listFolder)

	addTool(s, toolSpec{
		name:        toolStat,
		description: "Get the metadata of a file or folder: type, size, modification time, mimetype, etag and web_url. path is absolute.",
		readOnly:    true,
	}, s.statTool)

	addTool(s, toolSpec{
		name:        toolWebLink,
		description: "Get the link that opens a file or folder in the CERNBox web interface, to give to the user. path is absolute.",
		readOnly:    true,
	}, s.webLink)

	addTool(s, toolSpec{
		name: toolReadFile,
		description: fmt.Sprintf("Read a file. Text files (plain text, Markdown, CSV, JSON, YAML, XML, source code, notebooks) come back as UTF-8 text, "+
			"at most %d bytes per call: when truncated is true, call again with offset set to next_offset to read the next part. "+
			"Images (PNG, JPEG, GIF, WebP) up to %d bytes come back as an image. Other files, such as PDF or office documents, cannot be read: "+
			"the error carries a web_url to give to the user. path is absolute.", s.c.MaxReadBytes, s.c.MaxImageBytes),
		permissions: []string{scope.PermFilesRead},
		readOnly:    true,
	}, s.readFile)

	addTool(s, toolSpec{
		name: toolWriteFile,
		description: fmt.Sprintf("Create a file, or replace the whole content of an existing one. path is absolute and its folder must exist. "+
			"content is UTF-8 text, or base64 with encoding base64, at most %d bytes. "+
			"To change a file you read, pass its etag as if_match: the write then fails with precondition_failed if someone changed the file meanwhile, instead of overwriting their change.", s.c.MaxWriteBytes),
		permissions: []string{scope.PermFilesWrite},
		destructive: true,
		enums:       map[string][]any{"encoding": {"utf8", "base64"}},
	}, s.writeFile)

	addTool(s, toolSpec{
		name:        toolCreateFolder,
		description: "Create a folder. path is absolute and its parent must exist.",
		permissions: []string{scope.PermFilesWrite},
	}, s.createFolder)

	addTool(s, toolSpec{
		name:        toolMove,
		description: "Move or rename a file or folder. source and destination are absolute paths; destination is the new path of the item itself, not the folder to move it into.",
		permissions: []string{scope.PermFilesMove},
		destructive: true,
	}, s.move)

	addTool(s, toolSpec{
		name: toolDelete,
		description: "Delete a file or folder. It goes to the recycle bin, from where restore_from_trash can bring it back. " +
			"Pass the etag as if_match to delete only if the item did not change since you looked at it. path is absolute.",
		permissions: []string{scope.PermFilesDelete},
		destructive: true,
	}, s.delete)
}

type spaceEntry struct {
	Name   string `json:"name"`
	Type   string `json:"type" jsonschema:"home or project"`
	Path   string `json:"path"`
	ID     string `json:"id"`
	WebURL string `json:"web_url"`
}

type listSpacesIn struct{}

type listSpacesOut struct {
	Spaces []spaceEntry `json:"spaces"`
	Roots  []string     `json:"roots,omitempty" jsonschema:"the only folders this connection may access, when it is restricted"`
}

func (s *svc) listSpaces(c *call, _ listSpacesIn) (listSpacesOut, error) {
	res, err := c.gw.ListStorageSpaces(c.ctx, &provider.ListStorageSpacesRequest{})
	if err != nil {
		return listSpacesOut{}, asToolError(err, "spaces")
	}
	if err := checkStatus(res.GetStatus(), "spaces"); err != nil {
		return listSpacesOut{}, err
	}
	out := listSpacesOut{Spaces: []spaceEntry{}, Roots: c.ap.Roots}
	for _, sp := range res.GetStorageSpaces() {
		p := sp.GetRootInfo().GetPath()
		if p == "" || !c.ap.UnderRoots(p) {
			continue
		}
		out.Spaces = append(out.Spaces, spaceEntry{
			Name:   sp.GetName(),
			Type:   sp.GetSpaceType(),
			Path:   p,
			ID:     sp.GetId().GetOpaqueId(),
			WebURL: s.webURL(p, sp.GetRootInfo().GetId()),
		})
	}
	return out, nil
}

type listFolderIn struct {
	Path   string `json:"path" jsonschema:"absolute path of the folder"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor of the previous page"`
	Limit  int    `json:"limit,omitempty" jsonschema:"most entries to return in this page"`
}

type listFolderOut struct {
	Folder     entry   `json:"folder"`
	Entries    []entry `json:"entries"`
	NextCursor string  `json:"next_cursor,omitempty" jsonschema:"set when there are more entries: pass it as cursor"`
}

func (s *svc) listFolder(c *call, in listFolderIn) (listFolderOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return listFolderOut{}, err
	}
	c.path = p
	info, err := c.stat(p)
	if err != nil {
		return listFolderOut{}, err
	}
	if info.GetType() != provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return listFolderOut{}, newError(codeInvalidArgument, p+" is a file, not a folder; use stat or read_file")
	}
	res, err := c.gw.ListContainer(c.ctx, &provider.ListContainerRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return listFolderOut{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return listFolderOut{}, err
	}
	infos, next, err := page(res.GetInfos(), (*provider.ResourceInfo).GetPath, in.Cursor, s.pageSize(in.Limit))
	if err != nil {
		return listFolderOut{}, err
	}
	out := listFolderOut{Folder: s.entry(info), Entries: make([]entry, 0, len(infos)), NextCursor: next}
	for _, i := range infos {
		out.Entries = append(out.Entries, s.entry(i))
	}
	return out, nil
}

type pathIn struct {
	Path string `json:"path" jsonschema:"absolute path"`
}

func (s *svc) statTool(c *call, in pathIn) (entry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return entry{}, err
	}
	c.path = p
	info, err := c.stat(p)
	if err != nil {
		return entry{}, err
	}
	return s.entry(info), nil
}

type webLinkOut struct {
	Path   string `json:"path"`
	WebURL string `json:"web_url"`
}

func (s *svc) webLink(c *call, in pathIn) (webLinkOut, error) {
	e, err := s.statTool(c, in)
	if err != nil {
		return webLinkOut{}, err
	}
	return webLinkOut{Path: e.Path, WebURL: e.WebURL}, nil
}

func (s *svc) createFolder(c *call, in pathIn) (entry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return entry{}, err
	}
	c.path = p
	res, err := c.gw.CreateContainer(c.ctx, &provider.CreateContainerRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return entry{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return entry{}, err
	}
	return s.statTool(c, in)
}

type moveIn struct {
	Source      string `json:"source" jsonschema:"absolute path of the item to move"`
	Destination string `json:"destination" jsonschema:"absolute path the item will have"`
}

func (s *svc) move(c *call, in moveIn) (entry, error) {
	src, err := c.absPath(in.Source, "source")
	if err != nil {
		return entry{}, err
	}
	dst, err := c.absPath(in.Destination, "destination")
	if err != nil {
		return entry{}, err
	}
	c.path, c.destination = src, dst
	res, err := c.gw.Move(c.ctx, &provider.MoveRequest{
		Source:      &provider.Reference{Path: src},
		Destination: &provider.Reference{Path: dst},
	})
	if err != nil {
		return entry{}, asToolError(err, src)
	}
	if err := checkStatus(res.GetStatus(), src); err != nil {
		return entry{}, err
	}
	info, err := c.stat(dst)
	if err != nil {
		return entry{}, err
	}
	return s.entry(info), nil
}

type deleteIn struct {
	Path    string `json:"path" jsonschema:"absolute path of the file or folder"`
	IfMatch string `json:"if_match,omitempty" jsonschema:"etag the item must still have"`
}

type deleteOut struct {
	Path    string `json:"path"`
	Deleted bool   `json:"deleted"`
}

func (s *svc) delete(c *call, in deleteIn) (deleteOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return deleteOut{}, err
	}
	c.path = p
	info, err := c.stat(p)
	if err != nil {
		return deleteOut{}, err
	}
	if in.IfMatch != "" && !sameEtag(in.IfMatch, info.GetEtag()) {
		return deleteOut{}, newError(codePreconditionFailed, fmt.Sprintf("%s changed: its etag is now %s", p, info.GetEtag()))
	}
	res, err := c.gw.Delete(c.ctx, &provider.DeleteRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return deleteOut{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return deleteOut{}, err
	}
	return deleteOut{Path: p, Deleted: true}, nil
}

type readFileIn struct {
	Path   string `json:"path" jsonschema:"absolute path of the file"`
	Offset int64  `json:"offset,omitempty" jsonschema:"byte offset to read from: 0, or next_offset of the previous call"`
	Length int64  `json:"length,omitempty" jsonschema:"most bytes to read; the server caps it"`
}

type readFileOut struct {
	Path          string `json:"path"`
	MimeType      string `json:"mimetype"`
	Etag          string `json:"etag"`
	Size          uint64 `json:"size" jsonschema:"size of the whole file in bytes"`
	Offset        int64  `json:"offset"`
	ReturnedBytes int64  `json:"returned_bytes"`
	Truncated     bool   `json:"truncated" jsonschema:"true when the file goes on after this part"`
	NextOffset    int64  `json:"next_offset,omitempty" jsonschema:"offset to read the next part from, when truncated"`
	Content       string `json:"content,omitempty" jsonschema:"the text read; absent for an image, which comes as image content"`
	WebURL        string `json:"web_url"`
}

// imageTypes are the images MCP clients show.
var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

var textTypes = map[string]bool{
	"application/javascript": true, "application/x-javascript": true, "application/ecmascript": true,
	"application/x-sh": true, "application/x-shellscript": true, "application/x-csh": true,
	"application/yaml": true, "application/x-yaml": true, "application/toml": true, "application/x-toml": true,
	"application/sql": true, "application/x-sql": true, "application/graphql": true,
	"application/x-tex": true, "application/x-latex": true, "application/x-perl": true, "application/x-ruby": true,
	"application/x-php": true, "application/x-httpd-php": true, "application/x-ndjson": true, "application/csv": true,
	"application/x-python": true, "application/x-ipynb": true,
}

func baseMimeType(mt string) string {
	mt, _, _ = strings.Cut(mt, ";")
	return strings.ToLower(strings.TrimSpace(mt))
}

func isText(mt string) bool {
	mt = baseMimeType(mt)
	return strings.HasPrefix(mt, "text/") || strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml") ||
		mt == "application/json" || mt == "application/xml" || textTypes[mt]
}

// looksLikeText tells text from binary content of an unknown type.
func looksLikeText(b []byte) bool {
	return !bytes.Contains(b, []byte{0}) && utf8.Valid(cutPartialRune(b))
}

// cutPartialRune drops the bytes of a rune cut off at the end of b.
func cutPartialRune(b []byte) []byte {
	for i := len(b) - 1; i >= 0 && i >= len(b)-utf8.UTFMax; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i]
			}
			break
		}
	}
	return b
}

func (s *svc) readFile(c *call, in readFileIn) (readFileOut, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return readFileOut{}, err
	}
	c.path = p
	info, err := c.stat(p)
	if err != nil {
		return readFileOut{}, err
	}
	if info.GetType() == provider.ResourceType_RESOURCE_TYPE_CONTAINER {
		return readFileOut{}, newError(codeInvalidArgument, p+" is a folder; use list_folder")
	}
	size := int64(info.GetSize())
	out := readFileOut{Path: p, MimeType: info.GetMimeType(), Etag: info.GetEtag(), Size: info.GetSize(), WebURL: s.webURL(p, info.GetId())}
	mt := baseMimeType(info.GetMimeType())

	if imageTypes[mt] {
		if size > s.c.MaxImageBytes {
			return readFileOut{}, &toolError{Code: codeTooLarge, Message: fmt.Sprintf("the image has %d bytes, more than the %d that can be returned", size, s.c.MaxImageBytes), WebURL: out.WebURL}
		}
		if err := s.takeReadBytes(c, size); err != nil {
			return readFileOut{}, err
		}
		data, err := s.download(c, p, 0, size)
		if err != nil {
			return readFileOut{}, err
		}
		c.bytes = int64(len(data))
		c.content = []sdk.Content{&sdk.ImageContent{Data: data, MIMEType: mt}}
		out.ReturnedBytes = c.bytes
		return out, nil
	}

	if !isText(mt) && size > 0 {
		text := false
		if mt == "" || mt == "application/octet-stream" {
			probe, err := s.download(c, p, 0, min(size, 512))
			if err != nil {
				return readFileOut{}, err
			}
			text = looksLikeText(probe)
		}
		if !text {
			return readFileOut{}, &toolError{Code: codeUnsupportedType, Message: fmt.Sprintf("%s files cannot be read here; give the user the web_url to open it", info.GetMimeType()), WebURL: out.WebURL}
		}
	}

	if in.Offset < 0 || in.Length < 0 {
		return readFileOut{}, newError(codeInvalidArgument, "offset and length cannot be negative")
	}
	offset := min(in.Offset, size)
	n := s.c.MaxReadBytes
	if in.Length > 0 {
		n = min(n, in.Length)
	}
	n = min(n, size-offset)
	var data []byte
	if n > 0 {
		if err := s.takeReadBytes(c, n); err != nil {
			return readFileOut{}, err
		}
		if data, err = s.download(c, p, offset, n); err != nil {
			return readFileOut{}, err
		}
	}
	if offset+int64(len(data)) < size {
		data = cutPartialRune(data)
	}
	c.bytes = int64(len(data))
	out.Offset = offset
	out.ReturnedBytes = int64(len(data))
	out.Content = strings.ToValidUTF8(string(data), "�")
	if end := offset + out.ReturnedBytes; end < size {
		out.Truncated = true
		out.NextOffset = end
	}
	return out, nil
}

// download reads n bytes of the file p from offset, through the data gateway.
func (s *svc) download(c *call, p string, offset, n int64) ([]byte, error) {
	res, err := c.gw.InitiateFileDownload(c.ctx, &provider.InitiateFileDownloadRequest{Ref: &provider.Reference{Path: p}})
	if err != nil {
		return nil, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return nil, err
	}
	var endpoint, token string
	for _, proto := range res.GetProtocols() {
		if proto.GetProtocol() == "simple" {
			endpoint, token = proto.GetDownloadEndpoint(), proto.GetToken()
		}
	}
	if endpoint == "" {
		return nil, newError(codeInternal, "no download protocol for "+p)
	}
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, newError(codeInternal, err.Error())
	}
	req.Header.Set(datagateway.TokenTransportHeader, token)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+n-1))
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, newError(codeInternal, "downloading "+p+": "+err.Error())
	}
	defer resp.Body.Close()
	if err := checkHTTP(resp.StatusCode, p); err != nil {
		return nil, err
	}
	body := io.Reader(resp.Body)
	if resp.StatusCode != http.StatusPartialContent && offset > 0 {
		// The range was ignored.
		if _, err := io.CopyN(io.Discard, body, offset); err != nil {
			return nil, newError(codeInternal, "downloading "+p+": "+err.Error())
		}
	}
	data, err := io.ReadAll(io.LimitReader(body, n))
	if err != nil {
		return nil, newError(codeInternal, "downloading "+p+": "+err.Error())
	}
	return data, nil
}

type writeFileIn struct {
	Path     string `json:"path" jsonschema:"absolute path of the file"`
	Content  string `json:"content" jsonschema:"the whole new content of the file"`
	Encoding string `json:"encoding,omitempty" jsonschema:"utf8 (the default) or base64"`
	IfMatch  string `json:"if_match,omitempty" jsonschema:"etag the file must still have; recommended when replacing a file"`
}

func (s *svc) writeFile(c *call, in writeFileIn) (entry, error) {
	p, err := c.absPath(in.Path, "path")
	if err != nil {
		return entry{}, err
	}
	c.path = p
	data := []byte(in.Content)
	if in.Encoding == "base64" {
		if data, err = base64.StdEncoding.DecodeString(in.Content); err != nil {
			return entry{}, newError(codeInvalidArgument, "content is not valid base64: "+err.Error())
		}
	}
	if int64(len(data)) > s.c.MaxWriteBytes {
		return entry{}, newError(codeTooLarge, fmt.Sprintf("the content has %d bytes, more than the %d a file can be written with", len(data), s.c.MaxWriteBytes))
	}
	if in.IfMatch != "" {
		info, err := c.stat(p)
		if errorCode(err) == codeNotFound {
			return entry{}, newError(codePreconditionFailed, p+" does not exist; omit if_match to create it")
		}
		if err != nil {
			return entry{}, err
		}
		if !sameEtag(in.IfMatch, info.GetEtag()) {
			return entry{}, newError(codePreconditionFailed, fmt.Sprintf("%s changed since you read it: its etag is now %s; read it again", p, info.GetEtag()))
		}
	}

	length := strconv.Itoa(len(data))
	res, err := c.gw.InitiateFileUpload(c.ctx, &provider.InitiateFileUploadRequest{
		Ref:    &provider.Reference{Path: p},
		Opaque: &types.Opaque{Map: map[string]*types.OpaqueEntry{"Upload-Length": {Decoder: "plain", Value: []byte(length)}}},
	})
	if err != nil {
		return entry{}, asToolError(err, p)
	}
	if err := checkStatus(res.GetStatus(), p); err != nil {
		return entry{}, err
	}
	var endpoint, token string
	for _, proto := range res.GetProtocols() {
		if proto.GetProtocol() == "simple" {
			endpoint, token = proto.GetUploadEndpoint(), proto.GetToken()
		}
	}
	if endpoint == "" {
		return entry{}, newError(codeInternal, "no upload protocol for "+p)
	}
	req, err := http.NewRequestWithContext(c.ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return entry{}, newError(codeInternal, err.Error())
	}
	req.Header.Set(datagateway.TokenTransportHeader, token)
	req.Header.Set("Upload-Length", length)
	resp, err := s.client.Do(req)
	if err != nil {
		return entry{}, newError(codeInternal, "uploading "+p+": "+err.Error())
	}
	defer resp.Body.Close()
	if err := checkHTTP(resp.StatusCode, p); err != nil {
		return entry{}, err
	}
	c.bytes = int64(len(data))
	info, err := c.stat(p)
	if err != nil {
		return entry{}, err
	}
	return s.entry(info), nil
}
