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
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
	tusd "github.com/tus/tusd/pkg/handler"
)

func (s *svc) handlePathTusPost(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "tus-post")
	defer span.End()

	// read filename from metadata
	meta := tusd.ParseMetadataHeader(r.Header.Get(HeaderUploadMetadata))
	for _, r := range nameRules {
		if !r.Test(meta["filename"]) {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
	}

	// append filename to current dir
	fn := path.Join(ns, r.URL.Path, meta["filename"])

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	// check tus headers?

	ref := &provider.Reference{Path: fn}
	s.handleTusPost(ctx, w, r, meta, ref, sublog)
}

func (s *svc) handleSpacesTusPost(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "spaces-tus-post")
	defer span.End()

	// read filename from metadata
	meta := tusd.ParseMetadataHeader(r.Header.Get(HeaderUploadMetadata))
	if meta["filename"] == "" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()

	spaceRef, status, err := s.lookUpStorageSpaceReference(ctx, spaceID, path.Join(r.URL.Path, meta["filename"]))
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handleTusPost(ctx, w, r, meta, spaceRef, sublog)
}

func (s *svc) handleTusPost(ctx context.Context, w http.ResponseWriter, r *http.Request, meta map[string]string, ref *provider.Reference, log zerolog.Logger) {
	w.Header().Add(HeaderAccessControlAllowHeaders, strings.Join([]string{HeaderTusResumable, HeaderUploadLength, HeaderUploadMetadata, HeaderIfMatch}, ", "))
	w.Header().Add(HeaderAccessControlExposeHeaders, strings.Join([]string{HeaderTusResumable, HeaderLocation}, ", "))
	w.Header().Set(HeaderTusExtension, "creation,creation-with-upload,checksum,expiration")

	w.Header().Set(HeaderTusResumable, "1.0.0")

	// Test if the version sent by the client is supported
	// GET methods are not checked since a browser may visit this URL and does
	// not include this header. This request is not part of the specification.
	if r.Header.Get(HeaderTusResumable) != "1.0.0" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if r.Header.Get(HeaderUploadLength) == "" {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	// r.Header.Get("OC-Checksum")
	// TODO must be SHA1, ADLER32 or MD5 ... in capital letters????
	// curl -X PUT https://demo.owncloud.com/remote.php/webdav/testcs.bin -u demo:demo -d '123' -v -H 'OC-Checksum: SHA1:40bd001563085fc35165329ea1ff5c5ecbdbbeef'

	// TODO check Expect: 100-continue

	// check if destination exists or is a file
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{
		Ref: ref,
	}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK && sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&log, w, sRes.Status)
		return
	}

	info := sRes.Info
	if info != nil && info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
		log.Warn().Msg("resource is not a file")
		w.WriteHeader(http.StatusConflict)
		return
	}

	if info != nil {
		clientETag := r.Header.Get(HeaderIfMatch)
		serverETag := info.Etag
		if clientETag != "" {
			if clientETag != serverETag {
				log.Warn().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg("etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	opaqueMap := map[string]*typespb.OpaqueEntry{
		HeaderUploadLength: {
			Decoder: "plain",
			Value:   []byte(r.Header.Get(HeaderUploadLength)),
		},
	}

	mtime := meta["mtime"]
	if mtime != "" {
		opaqueMap[HeaderOCMtime] = &typespb.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(mtime),
		}
	}

	// initiateUpload
	uReq := &provider.InitiateFileUploadRequest{
		Ref: ref,
		Opaque: &typespb.Opaque{
			Map: opaqueMap,
		},
	}

	uRes, err := client.InitiateFileUpload(ctx, uReq)
	if err != nil {
		log.Error().Err(err).Msg("error initiating file upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		if uRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		HandleErrorStatus(&log, w, uRes.Status)
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

	w.Header().Set(HeaderLocation, ep)

	// for creation-with-upload extension forward bytes to dataprovider
	// TODO check this really streams
	if r.Header.Get(HeaderContentType) == "application/offset+octet-stream" {
		length, err := strconv.ParseInt(r.Header.Get(HeaderContentLength), 10, 64)
		if err != nil {
			log.Debug().Err(err).Msg("wrong request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var httpRes *http.Response

		httpReq, err := rhttp.NewRequest(ctx, http.MethodPatch, ep, r.Body)
		if err != nil {
			log.Debug().Err(err).Msg("wrong request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		httpReq.Header.Set(HeaderContentType, r.Header.Get(HeaderContentType))
		httpReq.Header.Set(HeaderContentLength, r.Header.Get(HeaderContentLength))
		if r.Header.Get(HeaderUploadOffset) != "" {
			httpReq.Header.Set(HeaderUploadOffset, r.Header.Get(HeaderUploadOffset))
		} else {
			httpReq.Header.Set(HeaderUploadOffset, "0")
		}
		httpReq.Header.Set(HeaderTusResumable, r.Header.Get(HeaderTusResumable))

		httpRes, err = s.client.Do(httpReq)
		if err != nil {
			log.Error().Err(err).Msg("error doing GET request to data service")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer httpRes.Body.Close()

		w.Header().Set(HeaderUploadOffset, httpRes.Header.Get(HeaderUploadOffset))
		w.Header().Set(HeaderTusResumable, httpRes.Header.Get(HeaderTusResumable))
		w.Header().Set(HeaderTusUploadExpires, httpRes.Header.Get(HeaderTusUploadExpires))
		if httpRes.StatusCode != http.StatusNoContent {
			w.WriteHeader(httpRes.StatusCode)
			return
		}

		// check if upload was fully completed
		if length == 0 || httpRes.Header.Get(HeaderUploadOffset) == r.Header.Get(HeaderUploadLength) {
			// get uploaded file metadata

			sRes, err := client.Stat(ctx, sReq)
			if err != nil {
				log.Error().Err(err).Msg("error sending grpc stat request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			if sRes.Status.Code != rpc.Code_CODE_OK && sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
				if sRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
					// the token expired during upload, so the stat failed
					// and we can't do anything about it.
					// the clients will handle this gracefully by doing a propfind on the file
					w.WriteHeader(http.StatusOK)
					return
				}

				HandleErrorStatus(&log, w, sRes.Status)
				return
			}

			info := sRes.Info
			if info == nil {
				log.Error().Msg("No info found for uploaded file")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if httpRes != nil && httpRes.Header != nil && httpRes.Header.Get(HeaderOCMtime) != "" {
				// set the "accepted" value if returned in the upload response headers
				w.Header().Set(HeaderOCMtime, httpRes.Header.Get(HeaderOCMtime))
			}

			// get WebDav permissions for file
			isPublic := false
			if info.Opaque != nil && info.Opaque.Map != nil {
				if info.Opaque.Map["link-share"] != nil && info.Opaque.Map["link-share"].Decoder == "json" {
					ls := &link.PublicShare{}
					_ = json.Unmarshal(info.Opaque.Map["link-share"].Value, ls)
					isPublic = ls != nil
				}
			}
			isShared := !isCurrentUserOwner(ctx, info.Owner)
			role := conversions.RoleFromResourcePermissions(info.PermissionSet)
			permissions := role.WebDAVPermissions(
				info.Type == provider.ResourceType_RESOURCE_TYPE_CONTAINER,
				isShared,
				false,
				isPublic,
			)

			w.Header().Set(HeaderContentType, info.MimeType)
			w.Header().Set(HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(info.Id))
			w.Header().Set(HeaderOCETag, info.Etag)
			w.Header().Set(HeaderETag, info.Etag)
			w.Header().Set(HeaderOCPermissions, permissions)

			t := utils.TSToTime(info.Mtime).UTC()
			lastModifiedString := t.Format(time.RFC1123Z)
			w.Header().Set(HeaderLastModified, lastModifiedString)
		}
	}

	w.WriteHeader(http.StatusCreated)
}
