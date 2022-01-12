// Copyright 2018-2022 CERN
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

package net

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
)
