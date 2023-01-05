// Copyright 2018-2023 CERN
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

// Handler handles requests.
func (h *WebDavHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ns := applyLayout(r.Context(), h.namespace, h.useLoggedInUserNS, r.URL.Path)
		switch r.Method {
		case MethodPropfind:
			s.handlePathPropfind(w, r, ns)
		case MethodLock:
			s.handleLock(w, r, ns)
		case MethodUnlock:
			s.handleUnlock(w, r, ns)
		case MethodProppatch:
			s.handlePathProppatch(w, r, ns)
		case MethodMkcol:
			s.handlePathMkcol(w, r, ns)
		case MethodMove:
			s.handlePathMove(w, r, ns)
		case MethodCopy:
			s.handlePathCopy(w, r, ns)
		case MethodReport:
			s.handleReport(w, r, ns)
		case http.MethodGet:
			s.handlePathGet(w, r, ns)
		case http.MethodPut:
			s.handlePathPut(w, r, ns)
		case http.MethodPost:
			s.handlePathTusPost(w, r, ns)
		case http.MethodOptions:
			s.handleOptions(w, r)
		case http.MethodHead:
			s.handlePathHead(w, r, ns)
		case http.MethodDelete:
			s.handlePathDelete(w, r, ns)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
