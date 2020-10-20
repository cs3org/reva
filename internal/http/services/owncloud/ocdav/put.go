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
	"regexp"
	"strconv"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/internal/http/utils"
	"github.com/cs3org/reva/pkg/appctx"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/eventials/go-tus"
	"github.com/eventials/go-tus/memorystore"
)

func isChunked(fn string) (bool, error) {
	// FIXME: also need to check whether the OC-Chunked header is set
	return regexp.MatchString(`-chunking-\w+-[0-9]+-[0-9]+$`, fn)
}

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
	log := appctx.GetLogger(ctx)

	ns = applyLayout(ctx, ns)

	fn := path.Join(ns, r.URL.Path)

	if r.Body == nil {
		log.Warn().Msg("body is nil")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ok, err := isChunked(fn)
	if err != nil {
		log.Error().Err(err).Msg("error checking if request is chunked or not")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if ok {
		// TODO: disable if chunking capability is turned off in config
		/**
		if s.c.Capabilities.Dav.Chunking == "1.0" {
			s.handlePutChunked(w, r)
		} else {
			log.Error().Err(err).Msg("chunking 1.0 is not enabled")
			w.WriteHeader(http.StatusBadRequest)
		}
		*/
		s.handlePutChunked(w, r)
		return
	}

	if isContentRange(r) {
		log.Warn().Msg("Content-Range not supported for PUT")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	if sufferMacOSFinder(r) {
		err := handleMacOSFinder(w, r)
		if err != nil {
			log.Error().Err(err).Msg("error handling Mac OS corner-case")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

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

	if sRes.Status.Code != rpc.Code_CODE_OK && sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		switch sRes.Status.Code {
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", fn).Interface("status", sRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", fn).Interface("status", sRes.Status).Msg("grpc stat request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
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

	length, err := strconv.ParseInt(r.Header.Get("Content-Length"), 10, 64)
	if err != nil {
		// Fallback to Upload-Length
		length, err = strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	opaqueMap := map[string]*typespb.OpaqueEntry{
		"Upload-Length": {
			Decoder: "plain",
			Value:   []byte(strconv.FormatInt(length, 10)),
		},
	}

	mtime := r.Header.Get("X-OC-Mtime")
	if mtime != "" {
		opaqueMap["X-OC-Mtime"] = &typespb.OpaqueEntry{
			Decoder: "plain",
			Value:   []byte(mtime),
		}

		// TODO: find a way to check if the storage really accepted the value
		w.Header().Set("X-OC-Mtime", "accepted")
	}

	uReq := &provider.InitiateFileUploadRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: fn},
		},
		Opaque: &typespb.Opaque{
			Map: opaqueMap,
		},
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
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", fn).Interface("status", uRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", fn).Interface("status", uRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", fn).Interface("status", uRes.Status).Msg("grpc initiate file upload request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	dataServerURL := uRes.UploadEndpoint

	// create the tus client.
	c := tus.DefaultConfig()
	c.Resume = true
	c.HttpClient = s.client

	c.Store, err = memorystore.NewMemoryStore()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("upload-endpoint", dataServerURL).
		Str("auth-header", tokenpkg.TokenHeader).
		Str("auth-token", tokenpkg.ContextMustGetToken(ctx)).
		Str("transfer-header", datagateway.TokenTransportHeader).
		Str("transfer-token", uRes.Token).
		Msg("adding tokens to headers")
	c.Header.Set(tokenpkg.TokenHeader, tokenpkg.ContextMustGetToken(ctx))
	c.Header.Set(datagateway.TokenTransportHeader, uRes.Token)

	tusc, err := tus.NewClient(dataServerURL, c)
	if err != nil {
		log.Error().Err(err).Msg("Could not get TUS client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metadata := map[string]string{
		"filename": path.Base(fn),
		"dir":      path.Dir(fn),
		//"checksum": fmt.Sprintf("%s %s", storageprovider.GRPC2PKGXS(xsType).String(), xs),
	}

	upload := tus.NewUpload(r.Body, length, metadata, "")

	// create the uploader.
	c.Store.Set(upload.Fingerprint, dataServerURL)
	uploader := tus.NewUploader(tusc, dataServerURL, upload, 0)

	// start the uploading process.
	err = uploader.Upload()
	if err != nil {
		log.Error().Err(err).Msg("Could not start TUS upload")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// stat again to check the new file's metadata
	sRes, err = client.Stat(ctx, sReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		switch sRes.Status.Code {
		case rpc.Code_CODE_NOT_FOUND:
			log.Debug().Str("path", fn).Interface("status", sRes.Status).Msg("resource not found")
			w.WriteHeader(http.StatusNotFound)
		case rpc.Code_CODE_PERMISSION_DENIED:
			log.Debug().Str("path", fn).Interface("status", sRes.Status).Msg("permission denied")
			w.WriteHeader(http.StatusForbidden)
		default:
			log.Error().Str("path", fn).Interface("status", sRes.Status).Msg("grpc stat request failed")
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	info2 := sRes.Info

	w.Header().Add("Content-Type", info2.MimeType)
	w.Header().Set("ETag", info2.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info2.Id))
	w.Header().Set("OC-ETag", info2.Etag)
	t := utils.TSToTime(info2.Mtime)
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
