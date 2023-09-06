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
	"context"
	"encoding/json"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	linkv1beta1 "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/notification/trigger"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
)

func sufferMacOSFinder(r *http.Request) bool {
	return r.Header.Get(HeaderExpectedEntityLength) != ""
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
	content := r.Header.Get(HeaderContentLength)
	expected := r.Header.Get(HeaderExpectedEntityLength)
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
	return r.Header.Get(HeaderContentRange) != ""
}

func (s *svc) handlePathPut(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	ref := &provider.Reference{Path: fn}

	s.handlePut(ctx, w, r, ref, sublog)
}

func (s *svc) handlePut(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, log zerolog.Logger) {
	if !checkPreconditions(w, r, log) {
		// checkPreconditions handles error returns
		return
	}

	length, err := getContentLength(w, r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &provider.StatRequest{Ref: ref}
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
	if info != nil {
		if info.Type != provider.ResourceType_RESOURCE_TYPE_FILE {
			log.Debug().Msg("resource is not a file")
			w.WriteHeader(http.StatusConflict)
			return
		}
		clientETag := r.Header.Get(HeaderIfMatch)
		serverETag := info.Etag
		if clientETag != "" {
			if clientETag != serverETag {
				log.Debug().Str("client-etag", clientETag).Str("server-etag", serverETag).Msg("etags mismatch")
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}
	}

	opaqueMap := map[string]*typespb.OpaqueEntry{
		HeaderUploadLength: {
			Decoder: "plain",
			Value:   []byte(strconv.FormatInt(length, 10)),
		},
	}

	if mtime := r.Header.Get(HeaderOCMtime); mtime != "" {
		opaqueMap[HeaderOCMtime] = &typespb.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(mtime),
		}

		// TODO: find a way to check if the storage really accepted the value
		w.Header().Set(HeaderOCMtime, "accepted")
	}

	// curl -X PUT https://demo.owncloud.com/remote.php/webdav/testcs.bin -u demo:demo -d '123' -v -H 'OC-Checksum: SHA1:40bd001563085fc35165329ea1ff5c5ecbdbbeef'

	var cparts []string
	// TUS Upload-Checksum header takes precedence
	if checksum := r.Header.Get(HeaderUploadChecksum); checksum != "" {
		cparts = strings.SplitN(checksum, " ", 2)
		if len(cparts) != 2 {
			log.Debug().Str("upload-checksum", checksum).Msg("invalid Upload-Checksum format, expected '[algorithm] [checksum]'")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Then try owncloud header
	} else if checksum := r.Header.Get(HeaderOCChecksum); checksum != "" {
		cparts = strings.SplitN(checksum, ":", 2)
		if len(cparts) != 2 {
			log.Debug().Str("oc-checksum", checksum).Msg("invalid OC-Checksum format, expected '[algorithm]:[checksum]'")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	// we do not check the algorithm here, because it might depend on the storage
	if len(cparts) == 2 {
		// Translate into TUS style Upload-Checksum header
		opaqueMap[HeaderUploadChecksum] = &typespb.OpaqueEntry{
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
		log.Error().Err(err).Msg("error initiating file upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if uRes.Status.Code != rpc.Code_CODE_OK {
		switch uRes.Status.Code {
		case rpc.Code_CODE_PERMISSION_DENIED:
			w.WriteHeader(http.StatusForbidden)
			b, err := Marshal(exception{
				code:    SabredavPermissionDenied,
				message: "permission denied: you have no permission to upload content",
			})
			HandleWebdavError(&log, w, b, err)
		case rpc.Code_CODE_NOT_FOUND:
			w.WriteHeader(http.StatusConflict)
		default:
			HandleErrorStatus(&log, w, uRes.Status)
		}
		return
	}

	var ep, token string
	for _, p := range uRes.Protocols {
		if p.Protocol == "simple" {
			ep, token = p.UploadEndpoint, p.Token
		}
	}

	httpReq, err := rhttp.NewRequest(ctx, http.MethodPut, ep, r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	httpRes, err := s.client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("error doing PUT request to data service")
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
				code:    SabredavBadRequest,
				message: "The computed checksum does not match the one received from the client.",
			})
			HandleWebdavError(&log, w, b, err)
			return
		}
		log.Error().Err(err).Msg("PUT request to data server failed")
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	ok, err := chunking.IsChunked(ref.Path)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if ok {
		chunk, err := chunking.GetChunkBLOBInfo(ref.Path)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sReq = &provider.StatRequest{Ref: &provider.Reference{Path: chunk.Path}}
	}

	// stat again to check the new file's metadata
	sRes, err = client.Stat(ctx, sReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&log, w, sRes.Status)
		return
	}

	newInfo := sRes.Info

	w.Header().Add(HeaderContentType, newInfo.MimeType)
	w.Header().Set(HeaderETag, newInfo.Etag)
	w.Header().Set(HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(newInfo.Id))
	w.Header().Set(HeaderOCETag, newInfo.Etag)
	t := utils.TSToTime(newInfo.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set(HeaderLastModified, lastModifiedString)

	var m map[string]*typespb.OpaqueEntry
	if sRes.Info.GetOpaque() != nil {
		m = sRes.Info.GetOpaque().Map
	}

	if ls, ok := m["link-share"]; ok {
		l := &linkv1beta1.PublicShare{}
		switch ls.Decoder {
		case "json":
			_ = json.Unmarshal(ls.Value, l)
		default:
			log.Error().Msgf("opaque entry decoder %s not recognized", ls.Decoder)
		}

		path := ""
		folder := ""
		_, shareFileName := filepath.Split(ref.Path)

		if f, ok := m["eos"]; ok {
			eosOpaque := make(map[string]interface{})
			switch f.Decoder {
			case "json":
				_ = json.Unmarshal(f.Value, &eosOpaque)
			default:
				log.Error().Msgf("opaque entry decoder %s not recognized", f.Decoder)
			}

			if p, ok := eosOpaque["file"]; ok {
				path, _ = filepath.Split(p.(string))
			}
		}

		if path != "" {
			folder = filepath.Base(path)
		}

		trg := &trigger.Trigger{
			Ref: l.Id.OpaqueId,
			TemplateData: map[string]interface{}{
				"path":     path,
				"folder":   folder,
				"fileName": shareFileName,
			},
		}
		s.notificationHelper.TriggerNotification(trg)
	}

	// file was new
	if info == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}

	// overwrite
	w.WriteHeader(http.StatusNoContent)
}

func (s *svc) handleSpacesPut(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()

	spaceRef, status, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handlePut(ctx, w, r, spaceRef, sublog)
}

func checkPreconditions(w http.ResponseWriter, r *http.Request, log zerolog.Logger) bool {
	if isContentRange(r) {
		log.Debug().Msg("Content-Range not supported for PUT")
		w.WriteHeader(http.StatusNotImplemented)
		return false
	}

	if sufferMacOSFinder(r) {
		err := handleMacOSFinder(w, r)
		if err != nil {
			log.Debug().Err(err).Msg("error handling Mac OS corner-case")
			w.WriteHeader(http.StatusInternalServerError)
			return false
		}
	}
	return true
}

func getContentLength(w http.ResponseWriter, r *http.Request) (int64, error) {
	length, err := strconv.ParseInt(r.Header.Get(HeaderContentLength), 10, 64)
	if err != nil {
		// Fallback to Upload-Length
		length, err = strconv.ParseInt(r.Header.Get(HeaderUploadLength), 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return length, nil
}
