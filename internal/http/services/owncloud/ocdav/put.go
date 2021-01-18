// Copyright 2018-2021 CERN
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
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/utils"
	"go.opencensus.io/trace"
)

func sufferMacOSFinder(r *http.Request) bool {
	return r.Header.Get("X-Expected-Entity-Length") != ""
}

func handleMacOSFinder(w http.ResponseWriter, r *http.Request) error {
	/*
	   Many webservers will not cooperate well with Finder PUT requests,
	   because it uses 'Chunked' transfer encoding for the request body.
	   The symptom of this problem is that Finder sends files to the
	   server, but they arrive as 0-length files.
	   If we don't do anything, the user might think they are uploading
	   files successfully, but they end up empty on the server. Instead,
	   we throw back an error if we detect this.
	   The reason Finder uses Chunked, is because it thinks the files
	   might change as it's being uploaded, and therefore the
	   Content-Length can vary.
	   Instead it sends the X-Expected-Entity-Length header with the size
	   of the file at the very start of the request. If this header is set,
	   but we don't get a request body we will fail the request to
	   protect the end-user.
	*/

	log := appctx.GetLogger(r.Context())
	content := r.Header.Get("Content-Length")
	expected := r.Header.Get("X-Expected-Entity-Length")
	log.Warn().Str("content-length", content).Str("x-expected-entity-length", expected).Msg("Mac OS Finder corner-case detected")

	// The best mitigation to this problem is to tell users to not use crappy Finder.
	// Another possible mitigation is to change the use the value of X-Expected-Entity-Length header in the Content-Length header.
	expectedInt, err := strconv.ParseInt(expected, 10, 64)
	if err != nil {
		log.Error().Err(err).Msg("error parsing expected length")
		w.WriteHeader(http.StatusBadRequest)
		return err
	}
	r.ContentLength = expectedInt
	return nil
}

func isContentRange(r *http.Request) bool {
	/*
		   Content-Range is dangerous for PUT requests:  PUT per definition
		   stores a full resource.  draft-ietf-httpbis-p2-semantics-15 says
		   in section 7.6:
			 An origin server SHOULD reject any PUT request that contains a
			 Content-Range header field, since it might be misinterpreted as
			 partial content (or might be partial content that is being mistakenly
			 PUT as a full representation).  Partial content updates are possible
			 by targeting a separately identified resource with state that
			 overlaps a portion of the larger resource, or by using a different
			 method that has been specifically defined for partial updates (for
			 example, the PATCH method defined in [RFC5789]).
		   This clarifies RFC2616 section 9.6:
			 The recipient of the entity MUST NOT ignore any Content-*
			 (e.g. Content-Range) headers that it does not understand or implement
			 and MUST return a 501 (Not Implemented) response in such cases.
		   OTOH is a PUT request with a Content-Range currently the only way to
		   continue an aborted upload request and is supported by curl, mod_dav,
		   Tomcat and others.  Since some clients do use this feature which results
		   in unexpected behaviour (cf PEAR::HTTP_WebDAV_Client 1.0.1), we reject
		   all PUT requests with a Content-Range for now.
	*/
	return r.Header.Get("Content-Range") != ""
}

func (s *svc) handlePut(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	if r.Body == nil {
		sublog.Debug().Msg("body is nil")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if isContentRange(r) {
		sublog.Debug().Msg("Content-Range not supported for PUT")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	if sufferMacOSFinder(r) {
		err := handleMacOSFinder(w, r)
		if err != nil {
			sublog.Debug().Err(err).Msg("error handling Mac OS corner-case")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	length, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		// Fallback to Upload-Length
		length, err = strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	s.handlePutHelper(w, r, r.Body, fn, length)
}

func (s *svc) handlePutHelper(w http.ResponseWriter, r *http.Request, content io.Reader, fn string, length int64) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "put")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	sReq := &provider.StatRequest{Ref: ref}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if sRes.Status.Code != rpc.Code_CODE_OK && sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&sublog, w, sRes.Status)
		return
	}

	info := sRes.Info
	if info != nil {
		if info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
			sublog.Debug().Msg("resource is not a file")
			w.WriteHeader(http.StatusConflict)
			return
		}
		clientETag := r.Header.Get("If-Match")
		serverETag := info.Etag
		if clientETag != "" {
			if clientETag != serverETag {
				sublog.Debug().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg("etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	opaqueMap := map[string]*typespb.OpaqueEntry{
		"Upload-Length": {
			Decoder: "plain",
			Value:   []byte(strconv.FormatInt(length, 10)),
		},
	}

	if mtime := r.Header.Get("X-OC-Mtime"); mtime != "" {
		opaqueMap["X-OC-Mtime"] = &typespb.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(mtime),
		}

		// TODO: find a way to check if the storage really accepted the value
		w.Header().Set("X-OC-Mtime", "accepted")
	}

	// curl -X PUT https://demo.owncloud.com/remote.php/webdav/testcs.bin -u demo:demo -d '123' -v -H 'OC-Checksum: SHA1:40bd001563085fc35165329ea1ff5c5ecbdbbeef'

	var cparts []string
	// TUS Upload-Checksum header takes precedence
	if checksum := r.Header.Get("Upload-Checksum"); checksum != "" {
		cparts = strings.SplitN(checksum, " ", 2)
		if len(cparts) != 2 {
			sublog.Debug().Str("upload-checksum", checksum).Msg("invalid Upload-Checksum format, expected '[algorithm] [checksum]'")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Then try owncloud header
	} else if checksum := r.Header.Get("OC-Checksum"); checksum != "" {
		cparts = strings.SplitN(checksum, ":", 2)
		if len(cparts) != 2 {
			sublog.Debug().Str("oc-checksum", checksum).Msg("invalid OC-Checksum format, expected '[algorithm]:[checksum]'")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	// we do not check the algorithm here, because it might depend on the storage
	if len(cparts) == 2 {
		// Translate into TUS style Upload-Checksum header
		opaqueMap["Upload-Checksum"] = &typespb.OpaqueEntry{
			Decoder: "plain",
			// algorithm is always lowercase, checksum is separated by space
			Value: []byte(strings.ToLower(cparts[0]) + " " + cparts[1]),
		}
	}

	uReq := &provider.InitiateFileUploadRequest{
		Ref:    ref,
		Opaque: &typespb.Opaque{Map: opaqueMap},
	}

	// where to upload the file?
	uRes, err := client.InitiateFileUpload(ctx, uReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error initiating file upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, uRes.Status)
		return
	}

	var ep, token string
	for _, p := range uRes.Protocols {
		if p.Protocol == "simple" {
			ep, token = p.UploadEndpoint, p.Token
		}
	}

	if length > 0 {
		httpReq, err := rhttp.NewRequest(ctx, "PUT", ep, content)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		httpReq.Header.Set(datagateway.TokenTransportHeader, token)

		httpRes, err := s.client.Do(httpReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error doing PUT request to data service")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer httpRes.Body.Close()
		if httpRes.StatusCode != http.StatusOK {
			if httpRes.StatusCode == http.StatusPartialContent {
				w.WriteHeader(http.StatusPartialContent)
				return
			}
			if httpRes.StatusCode == errtypes.StatusChecksumMismatch {
				w.WriteHeader(http.StatusBadRequest)
				b, err := Marshal(exception{
					code:    SabredavMethodBadRequest,
					message: "The computed checksum does not match the one received from the client.",
				})
				if err != nil {
					sublog.Error().Msgf("error marshaling xml response: %s", b)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_, err = w.Write(b)
				if err != nil {
					sublog.Err(err).Msg("error writing response")
				}
				return
			}
			sublog.Error().Err(err).Msg("PUT request to data server failed")
			w.WriteHeader(httpRes.StatusCode)
			return
		}
	}

	ok, err := chunking.IsChunked(fn)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if ok {
		chunk, err := chunking.GetChunkBLOBInfo(fn)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sReq = &provider.StatRequest{
			Ref: &provider.Reference{
				Spec: &provider.Reference_Path{
					Path: chunk.Path,
				},
			},
		}
	}

	// stat again to check the new file's metadata
	sRes, err = client.Stat(ctx, sReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, sRes.Status)
		return
	}

	newInfo := sRes.Info

	w.Header().Add("Content-Type", newInfo.MimeType)
	w.Header().Set("ETag", newInfo.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(newInfo.Id))
	w.Header().Set("OC-ETag", newInfo.Etag)
	t := utils.TSToTime(newInfo.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set("Last-Modified", lastModifiedString)

	// file was new
	if info == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}

	// overwrite
	w.WriteHeader(http.StatusNoContent)
}
