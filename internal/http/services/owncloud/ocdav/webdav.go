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
	"fmt"
	"net/http"
	"path"

	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rhttp/mux"
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

func (h *WebDavHandler) withNs(fn func(w http.ResponseWriter, r *http.Request, ns string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, _ := mux.ParamsFromRequest(r).Get("path")
		fmt.Println("************ withNs path", path)
		contextUser, ok := ctxpkg.ContextGetUser(r.Context())
		fmt.Println("************ USER IN CONTEXT", contextUser, ok)
		ns := applyLayout(r.Context(), h.namespace, h.useLoggedInUserNS, path)
		fn(w, r, ns)
	})
}

// Handler handles requests.
func (h *WebDavHandler) Handler(s *svc) http.Handler {
	router := mux.NewServeMux()
	router.Method(MethodPropfind, "/*path", h.withNs(s.handlePathPropfind))
	router.Method(MethodLock, "/*path", h.withNs(s.handleLock))
	router.Method(MethodUnlock, "/*path", h.withNs(s.handleUnlock))
	router.Method(MethodProppatch, "/*path", h.withNs(s.handlePathProppatch))
	router.Method(MethodMkcol, "/*path", h.withNs(s.handlePathMkcol))
	router.Method(MethodMove, "/*path", h.withNs(s.handlePathMove))
	router.Method(MethodCopy, "/*path", h.withNs(s.handlePathCopy))
	router.Method(MethodReport, "/*path", h.withNs(s.handleReport))
	router.Method(http.MethodGet, "/*path", h.withNs(s.handlePathGet))
	router.Method(http.MethodPut, "/*path", h.withNs(s.handlePathPut))
	router.Method(http.MethodPost, "/*path", h.withNs(s.handlePathTusPost))
	router.Method(http.MethodOptions, "/*path", http.HandlerFunc(s.handleOptions))
	router.Method(http.MethodHead, "/*path", h.withNs(s.handlePathHead))
	router.Method(http.MethodDelete, "/*path", h.withNs(s.handlePathDelete))
	return router
}
