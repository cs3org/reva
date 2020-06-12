// Copyright 2018-2020 CERN
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
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/utils"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	tusd "github.com/tus/tusd/pkg/handler"
)

func (s *svc) handleTusPost(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

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
	//TODO check Expect: 100-continue

	// read filename from metadata
	meta := tusd.ParseMetadataHeader(r.Header.Get("Upload-Metadata"))
	if meta["filename"] == "" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	ns = applyLayout(ctx, ns)

	// append filename to current dir
	fn := path.Join(ns, r.URL.Path, meta["filename"])

	// check tus headers?

	// check if destination exists or is a file
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
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
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		if sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	info := sRes.Info
	if info != nil && info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		log.Warn().Msg("resource is not a file")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if info != nil {
		clientETag := r.Header.Get("If-Match")
		serverETag := info.Etag
		if clientETag != "" {
			if clientETag != serverETag {
				log.Warn().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg("etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	// initiateUpload

	uReq := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
		Opaque: &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"Upload-Length": {
					Decoder: "plain",
					Value:   []byte(r.Header.Get("Upload-Length")),
				},
			},
		},
	}

	uRes, err := client.InitiateFileUpload(ctx, uReq)
	if err != nil {
		log.Error().Err(err).Msg("error initiating file upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", uRes.UploadEndpoint)

	// for creation-with-upload extension forward bytes to dataprovider
	// TODO check this really streams
	if r.Header.Get("Content-Type") == "application/offset+octet-stream" {

		httpClient := rhttp.GetHTTPClient(ctx)
		httpReq, err := rhttp.NewRequest(ctx, "PATCH", uRes.UploadEndpoint, r.Body)
		if err != nil {
			log.Err(err).Msg("wrong request")
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

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			log.Err(err).Msg("error doing GET request to data service")
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

		// check if upload was fully completed
		if httpRes.Header.Get("Upload-Offset") == r.Header.Get("Upload-Length") {
			// apply mtime to the metadata if specified
			mtime := meta["mtime"]
			if mtime != "" {
				sreq := &provider.SetArbitraryMetadataRequest{
					Ref: &provider.Reference{
						Spec: &provider.Reference_Path{Path: fn},
					},
					ArbitraryMetadata: &provider.ArbitraryMetadata{
						Metadata: map[string]string{},
					},
				}
				sreq.ArbitraryMetadata.Metadata["mtime"] = mtime
				res, err := client.SetArbitraryMetadata(ctx, sreq)
				if err != nil {
					log.Error().Err(err).
						Str("mtime", mtime).
						Msg("error sending a grpc SetArbitraryMetadata request for setting mtime")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if res.Status.Code != rpc.Code_CODE_OK {
					log.Error().Err(err).
						Str("mtime", mtime).
						Msgf("error sending a grpc SetArbitraryMetadata request for setting mtime, status %d", res.Status.Code)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			// get uploaded file metadata
			sRes, err := client.Stat(ctx, sReq)
			if err != nil {
				log.Error().Err(err).Msg("error sending grpc stat request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if sRes.Status.Code != rpc.Code_CODE_OK {
				if sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
			}

			info := sRes.Info
			if info == nil {
				log.Error().Str("fn", fn).Msg("No info found for uploaded file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", info.MimeType)
			w.Header().Set("OC-FileId", wrapResourceID(info.Id))
			w.Header().Set("OC-ETag", info.Etag)
			w.Header().Set("ETag", info.Etag)
			t := utils.TSToTime(info.Mtime)
			lastModifiedString := t.Format(time.RFC1123Z)
			w.Header().Set("Last-Modified", lastModifiedString)
		}
	}
	w.WriteHeader(http.StatusCreated)
}
