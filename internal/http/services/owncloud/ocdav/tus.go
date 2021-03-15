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
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/utils"
	tusd "github.com/tus/tusd/pkg/handler"
	"go.opencensus.io/trace"
)

func (s *svc) handleTusPost(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "tus-post")
	defer span.End()

	w.Header().Add("Access-Control-Allow-Headers", "Tus-Resumable, Upload-Length, Upload-Metadata, If-Match")
	w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Location")

	w.Header().Set("Tus-Resumable", "1.0.0")

	// Test if the version sent by the client is supported
	// GET methods are not checked since a browser may visit this URL and does
	// not include this header. This request is not part of the specification.
	if r.Header.Get("Tus-Resumable") != "1.0.0" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if r.Header.Get("Upload-Length") == "" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	// r.Header.Get("OC-Checksum")
	// TODO must be SHA1, ADLER32 or MD5 ... in capital letters????
	// curl -X PUT https://demo.owncloud.com/remote.php/webdav/testcs.bin -u demo:demo -d '123' -v -H 'OC-Checksum: SHA1:40bd001563085fc35165329ea1ff5c5ecbdbbeef'

	// TODO check Expect: 100-continue

	// read filename from metadata
	meta := tusd.ParseMetadataHeader(r.Header.Get("Upload-Metadata"))
	if meta["filename"] == "" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	// append filename to current dir
	fn := path.Join(ns, r.URL.Path, meta["filename"])

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	// check tus headers?

	// check if destination exists or is a file
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
	}
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
	if info != nil && info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		sublog.Warn().Msg("resource is not a file")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if info != nil {
		clientETag := r.Header.Get("If-Match")
		serverETag := info.Etag
		if clientETag != "" {
			if clientETag != serverETag {
				sublog.Warn().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg("etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	opaqueMap := map[string]*typespb.OpaqueEntry{
		"Upload-Length": {
			Decoder: "plain",
			Value:   []byte(r.Header.Get("Upload-Length")),
		},
	}

	mtime := meta["mtime"]
	if mtime != "" {
		opaqueMap["X-OC-Mtime"] = &typespb.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(mtime),
		}
	}

	// initiateUpload
	uReq := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
		Opaque: &typespb.Opaque{
			Map: opaqueMap,
		},
	}

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
		if p.Protocol == "tus" {
			ep, token = p.UploadEndpoint, p.Token
		}
	}

	// TUS clients don't understand the reva transfer token. We need to append it to the upload endpoint.
	// The DataGateway has to take care of pulling it back into the request header upon request arrival.
	if token != "" {
		if !strings.HasSuffix(ep, "/") {
			ep += "/"
		}
		ep += token
	}

	w.Header().Set("Location", ep)

	// for creation-with-upload extension forward bytes to dataprovider
	// TODO check this really streams
	if r.Header.Get("Content-Type") == "application/offset+octet-stream" {

		length, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			sublog.Debug().Err(err).Msg("wrong request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var httpRes *http.Response

		if length != 0 {
			httpReq, err := rhttp.NewRequest(ctx, "PATCH", ep, r.Body)
			if err != nil {
				sublog.Debug().Err(err).Msg("wrong request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			httpReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))
			httpReq.Header.Set("Content-Length", r.Header.Get("Content-Length"))
			if r.Header.Get("Upload-Offset") != "" {
				httpReq.Header.Set("Upload-Offset", r.Header.Get("Upload-Offset"))
			} else {
				httpReq.Header.Set("Upload-Offset", "0")
			}
			httpReq.Header.Set("Tus-Resumable", r.Header.Get("Tus-Resumable"))

			httpRes, err = s.client.Do(httpReq)
			if err != nil {
				sublog.Error().Err(err).Msg("error doing GET request to data service")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer httpRes.Body.Close()

			w.Header().Set("Upload-Offset", httpRes.Header.Get("Upload-Offset"))
			w.Header().Set("Tus-Resumable", httpRes.Header.Get("Tus-Resumable"))
			if httpRes.StatusCode != http.StatusNoContent {
				w.WriteHeader(httpRes.StatusCode)
				return
			}
		} else {
			sublog.Debug().Msg("Skipping sending a Patch request as body is empty")
		}

		// check if upload was fully completed
		if length == 0 || httpRes.Header.Get("Upload-Offset") == r.Header.Get("Upload-Length") {
			// get uploaded file metadata
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
			if info == nil {
				sublog.Error().Msg("No info found for uploaded file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if httpRes != nil && httpRes.Header != nil && httpRes.Header.Get("X-OC-Mtime") != "" {
				// set the "accepted" value if returned in the upload response headers
				w.Header().Set("X-OC-Mtime", httpRes.Header.Get("X-OC-Mtime"))
			}

			w.Header().Set("Content-Type", info.MimeType)
			w.Header().Set("OC-FileId", wrapResourceID(info.Id))
			w.Header().Set("OC-ETag", info.Etag)
			w.Header().Set("ETag", info.Etag)
			t := utils.TSToTime(info.Mtime).UTC()
			lastModifiedString := t.Format(time.RFC1123Z)
			w.Header().Set("Last-Modified", lastModifiedString)
		}
	}

	w.WriteHeader(http.StatusCreated)
}
