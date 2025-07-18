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

// Package download provides a library to handle file download requests.
package download

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/storage"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/rs/zerolog"
)

// GetOrHeadFile returns the requested file content.
func GetOrHeadFile(w http.ResponseWriter, r *http.Request, fs storage.FS, spaceID string) {
	ctx := r.Context()
	sublog := appctx.GetLogger(ctx).With().Str("svc", "datatx").Str("handler", "download").Logger()

	var fn string
	files, ok := r.URL.Query()["filename"]
	if !ok || len(files[0]) < 1 {
		fn = r.URL.Path
	} else {
		fn = files[0]
	}

	var ref *provider.Reference
	if spaceID == "" {
		// ensure the absolute path starts with '/'
		ref = &provider.Reference{Path: path.Join("/", fn)}
	} else {
		// build a storage space reference
		storageid, opaqeid, err := utils.SplitStorageSpaceID(spaceID)
		if err != nil {
			sublog.Error().Str("space_id", spaceID).Str("path", fn).Msg("invalid reference")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ref = &provider.Reference{
			ResourceId: &provider.ResourceId{StorageId: storageid, OpaqueId: opaqeid},
			// ensure the relative path starts with '.'
			Path: utils.MakeRelativePath(fn),
		}
	}
	// TODO check preconditions like If-Range, If-Match ...

	var (
		md      *provider.ResourceInfo
		content io.ReadCloser
		size    int64
		err     error
	)

	// do a stat to get the mime type
	if md, err = fs.GetMD(ctx, ref, nil); err != nil {
		handleError(w, &sublog, err, "stat")
		return
	}
	mimeType := md.MimeType

	if versionKey := r.URL.Query().Get("version_key"); versionKey != "" {
		// the request is for a version file
		stat, err := statRevision(ctx, fs, ref, versionKey)
		if err != nil {
			handleError(w, &sublog, err, "stat revision")
			return
		}
		size = int64(stat.Size)
		content, err = fs.DownloadRevision(ctx, ref, versionKey)
		if err != nil {
			handleError(w, &sublog, err, "download revision")
			return
		}
	} else {
		size = int64(md.Size)
		content, err = fs.Download(ctx, ref)
		if err != nil {
			handleError(w, &sublog, err, "download")
			return
		}
	}
	defer content.Close()

	var ranges []HTTPRange

	if r.Header.Get("Range") != "" {
		ranges, err = ParseRange(r.Header.Get("Range"), size)
		if err != nil {
			if err == ErrNoOverlap {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
			}
			sublog.Error().Err(err).Interface("md", md).Interface("ranges", ranges).Msg("range request not satisfiable")
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)

			return
		}
		if SumRangesSize(ranges) > size {
			// The total number of bytes in all the ranges
			// is larger than the size of the file by
			// itself, so this is probably an attack, or a
			// dumb client. Ignore the range request.
			ranges = nil
		}
	}

	code := http.StatusOK
	sendSize := size
	var sendContent io.Reader = content

	var s io.Seeker
	if s, ok = content.(io.Seeker); ok {
		// tell clients they can send range requests
		w.Header().Set("Accept-Ranges", "bytes")
	}

	// If we want to adhere to the Range request, the content must be seekable
	// If the storage provider does not support seeking the content,
	// we ignore the Range request
	if s != nil && len(ranges) > 0 {
		sublog.Debug().Int64("start", ranges[0].Start).Int64("length", ranges[0].Length).Msg("range request")

		switch {
		case len(ranges) == 1:
			// RFC 7233, Section 4.1:
			// "If a single part is being transferred, the server
			// generating the 206 response MUST generate a
			// Content-Range header field, describing what range
			// of the selected representation is enclosed, and a
			// payload consisting of the range.
			// ...
			// A server MUST NOT generate a multipart response to
			// a request for a single range, since a client that
			// does not request multiple parts might not support
			// multipart responses."
			ra := ranges[0]
			if _, err := s.Seek(ra.Start, io.SeekStart); err != nil {
				sublog.Error().Err(err).Int64("start", ra.Start).Int64("length", ra.Length).Msg("content is not seekable")
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			sendSize = ra.Length
			code = http.StatusPartialContent
			w.Header().Set("Content-Range", ra.ContentRange(size))
		case len(ranges) > 1:
			sendSize = RangesMIMESize(ranges, mimeType, size)
			code = http.StatusPartialContent

			pr, pw := io.Pipe()
			mw := multipart.NewWriter(pw)
			w.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
			sendContent = pr
			defer pr.Close() // cause writing goroutine to fail and exit if CopyN doesn't finish.
			go func() {
				for _, ra := range ranges {
					part, err := mw.CreatePart(ra.MimeHeader(mimeType, size))
					if err != nil {
						_ = pw.CloseWithError(err) // CloseWithError always returns nil
						return
					}
					if _, err := s.Seek(ra.Start, io.SeekStart); err != nil {
						_ = pw.CloseWithError(err) // CloseWithError always returns nil
						return
					}
					if _, err := io.CopyN(part, content, ra.Length); err != nil {
						_ = pw.CloseWithError(err) // CloseWithError always returns nil
						return
					}
				}
				mw.Close()
				pw.Close()
			}()
		}
	}

	if w.Header().Get("Content-Encoding") == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(sendSize, 10))
	}

	w.WriteHeader(code)

	if r.Method != http.MethodHead {
		var c int64
		c, err = io.CopyN(w, sendContent, sendSize)
		if err != nil {
			sublog.Error().Err(err).Msg("error copying data to response")
			return
		}
		if c != sendSize {
			sublog.Error().Int64("copied", c).Int64("size", sendSize).Msg("copied vs size mismatch")
		}
	}
}

func statRevision(ctx context.Context, fs storage.FS, ref *provider.Reference, revisionKey string) (*provider.FileVersion, error) {
	versions, err := fs.ListRevisions(ctx, ref)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.Key == revisionKey {
			return v, nil
		}
	}
	return nil, errtypes.NotFound("version not found")
}

func handleError(w http.ResponseWriter, log *zerolog.Logger, err error, action string) {
	switch err.(type) {
	case errtypes.IsNotFound:
		log.Debug().Err(err).Str("action", action).Msg("file not found")
		w.WriteHeader(http.StatusNotFound)
	case errtypes.IsPermissionDenied:
		log.Debug().Err(err).Str("action", action).Msg("permission denied")
		w.WriteHeader(http.StatusForbidden)
	default:
		log.Error().Err(err).Str("action", action).Msg("unexpected error")
		w.WriteHeader(http.StatusInternalServerError)
	}
}
