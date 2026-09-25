// Copyright 2018-2024 CERN
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

package ocdav

import (
	"net/http"
	"path"
)

// Common Webdav methods.
//
// Unless otherwise noted, these are defined in RFC 4918 section 9.
const (
	MethodPropfind  = "PROPFIND"
	MethodLock      = "LOCK"
	MethodUnlock    = "UNLOCK"
	MethodProppatch = "PROPPATCH"
	MethodMkcol     = "MKCOL"
	MethodMove      = "MOVE"
	MethodCopy      = "COPY"
	MethodReport    = "REPORT"
)

// Common HTTP headers.
const (
	HeaderAcceptRanges               = "Accept-Ranges"
	HeaderAccessControlAllowHeaders  = "Access-Control-Allow-Headers"
	HeaderAccessControlExposeHeaders = "Access-Control-Expose-Headers"
	HeaderAccessControlAllowOrigin   = "Access-Control-Allow-Origin"
	HeaderContentDisposistion        = "Content-Disposition"
	HeaderContentLength              = "Content-Length"
	HeaderContentRange               = "Content-Range"
	HeaderContentType                = "Content-Type"
	HeaderETag                       = "ETag"
	HeaderLastModified               = "Last-Modified"
	HeaderLocation                   = "Location"
	HeaderRange                      = "Range"
	HeaderIfMatch                    = "If-Match"
	HeaderChecksum                   = "Digest"
)

// Non standard HTTP headers.
const (
	HeaderOCFileID             = "OC-FileId"
	HeaderOCETag               = "OC-ETag"
	HeaderOCChecksum           = "OC-Checksum"
	HeaderOCPermissions        = "OC-Perm"
	HeaderDepth                = "Depth"
	HeaderDav                  = "DAV"
	HeaderTusResumable         = "Tus-Resumable"
	HeaderTusVersion           = "Tus-Version"
	HeaderTusExtension         = "Tus-Extension"
	HeaderTusChecksumAlgorithm = "Tus-Checksum-Algorithm"
	HeaderTusUploadExpires     = "Upload-Expires"
	HeaderDestination          = "Destination"
	HeaderOverwrite            = "Overwrite"
	HeaderUploadChecksum       = "Upload-Checksum"
	HeaderUploadLength         = "Upload-Length"
	HeaderUploadMetadata       = "Upload-Metadata"
	HeaderUploadOffset         = "Upload-Offset"
	HeaderOCMtime              = "X-OC-Mtime"
	HeaderExpectedEntityLength = "X-Expected-Entity-Length"
	HeaderTransferAuth         = "TransferHeaderAuthorization"
	HeaderLockID               = "X-Lock-Id"
	HeaderLockHolder           = "X-Lock-Holder"
	HeaderDisableVersioning    = "X-Disable-Versioning"
)

// WebDavHandler implements a dav endpoint.
type WebDavHandler struct {
	namespace         string
	useLoggedInUserNS bool
}

func (h *WebDavHandler) init(ns string, useLoggedInUserNS bool) error {
	h.namespace = path.Join("/", ns)
	h.useLoggedInUserNS = useLoggedInUserNS
	return nil
}

// davOp is a WebDAV operation: it serves one method against a namespace.
type davOp func(*svc, http.ResponseWriter, *http.Request, string)

// davOps are the operations a WebDAV endpoint serves, in the order they are
// declared. Listing them here, rather than switching on the method inside the
// handler, is what puts each of them in the route table and lets the router
// refuse a method nobody serves.
var davOps = []struct {
	method string
	op     davOp
}{
	{MethodPropfind, (*svc).handlePathPropfind},
	{MethodProppatch, (*svc).handlePathProppatch},
	{MethodMkcol, (*svc).handlePathMkcol},
	{MethodMove, (*svc).handlePathMove},
	{MethodCopy, (*svc).handlePathCopy},
	{MethodReport, (*svc).handleReport},
	{MethodLock, (*svc).handleLock},
	{MethodUnlock, (*svc).handleUnlock},
	{http.MethodGet, (*svc).handlePathGet},
	{http.MethodHead, (*svc).handlePathHead},
	{http.MethodPut, (*svc).handlePathPut},
	{http.MethodPost, (*svc).handlePathTusPost},
	{http.MethodDelete, (*svc).handlePathDelete},
	{http.MethodOptions, func(s *svc, w http.ResponseWriter, r *http.Request, _ string) {
		s.handleOptions(w, r)
	}},
}

// namespace returns the namespace this endpoint serves, for the given request.
func (h *WebDavHandler) namespaceFor(r *http.Request) string {
	return applyLayout(r.Context(), h.namespace, h.useLoggedInUserNS, r.URL.Path)
}
