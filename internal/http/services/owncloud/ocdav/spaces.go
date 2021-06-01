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
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/storage/utils/chunking"
	"github.com/cs3org/reva/pkg/utils"
	"go.opencensus.io/trace"
)

// SpacesHandler handles trashbin requests
type SpacesHandler struct {
	gatewaySvc string
}

func (h *SpacesHandler) init(c *Config) error {
	h.gatewaySvc = c.GatewaySvc
	return nil
}

// Handler handles requests
func (h *SpacesHandler) Handler(s *svc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ctx := r.Context()
		// log := appctx.GetLogger(ctx)

		if r.Method == http.MethodOptions {
			s.handleOptions(w, r, "spaces")
			return
		}

		var spaceID string
		spaceID, r.URL.Path = router.ShiftPath(r.URL.Path)

		if spaceID == "" {
			// listing is disabled, no auth will change that
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		switch r.Method {
		case MethodPropfind:
			s.handleSpacesPropfind(w, r, spaceID)
		case MethodProppatch:
			s.handleSpacesProppatch(w, r, spaceID)
		case MethodLock:
			s.handleLock(w, r, spaceID)
		case MethodUnlock:
			s.handleUnlock(w, r, spaceID)
		case MethodMkcol:
			s.handleSpacesMkCol(w, r, spaceID)
		case MethodMove:
			s.handleSpacesMove(w, r, spaceID)
		case MethodCopy:
			s.handleSpacesCopy(w, r, spaceID)
		case MethodReport:
			s.handleReport(w, r, spaceID)
		case http.MethodGet:
			s.handleSpacesGet(w, r, spaceID)
		case http.MethodPut:
			s.handleSpacesPut(w, r, spaceID)
		case http.MethodPost:
			s.handleSpacesTusPost(w, r, spaceID)
		case http.MethodOptions:
			s.handleOptions(w, r, spaceID)
		case http.MethodHead:
			s.handleSpacesHead(w, r, spaceID)
		case http.MethodDelete:
			s.handleSpacesDelete(w, r, spaceID)
		default:
			http.Error(w, http.StatusText(http.StatusNotImplemented), http.StatusNotImplemented)
		}
	})
}

func (s *svc) lookUpStorageSpaceReference(ctx context.Context, spaceID string, relativePath string) (*storageProvider.Reference, *rpc.Status, error) {
	// Get the getway client
	gatewayClient, err := s.getClient()
	if err != nil {
		return nil, nil, err
	}

	// retrieve a specific storage space
	lSSReq := &storageProvider.ListStorageSpacesRequest{
		Filters: []*storageProvider.ListStorageSpacesRequest_Filter{
			{
				Type: storageProvider.ListStorageSpacesRequest_Filter_TYPE_ID,
				Term: &storageProvider.ListStorageSpacesRequest_Filter_Id{
					Id: &storageProvider.StorageSpaceId{
						OpaqueId: spaceID,
					},
				},
			},
		},
	}

	lSSRes, err := gatewayClient.ListStorageSpaces(ctx, lSSReq)
	if err != nil || lSSRes.Status.Code != rpc.Code_CODE_OK {
		return nil, lSSRes.Status, err
	}

	if len(lSSRes.StorageSpaces) != 1 {
		return nil, nil, fmt.Errorf("unexpected number of spaces")
	}
	space := lSSRes.StorageSpaces[0]

	// TODO:
	// Use ResourceId to make request to the actual storage provider via the gateway.
	// - Copy  the storageId from the storage space root
	// - set the opaque Id to /storageSpaceId/relativePath in
	// Correct fix would be to add a new Reference to the CS3API
	return &storageProvider.Reference{
		Spec: &storageProvider.Reference_Id{
			Id: &storageProvider.ResourceId{
				StorageId: space.Root.StorageId,
				OpaqueId:  filepath.Join("/", space.Root.OpaqueId, relativePath), // FIXME this is a hack to pass storage space id and a relative path to the storage provider
			},
		},
	}, lSSRes.Status, nil
}

func (s *svc) handleSpacesPut(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()

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

	s.handleSpacesPutHelper(w, r, r.Body, spaceRef, length)
}

func (s *svc) handleSpacesPutHelper(w http.ResponseWriter, r *http.Request, content io.Reader, ref *storageProvider.Reference, length int64) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "put")
	defer span.End()

	sublog := appctx.GetLogger(ctx)
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	sReq := &storageProvider.StatRequest{Ref: ref}
	sRes, err := client.Stat(ctx, sReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if sRes.Status.Code != rpc.Code_CODE_OK && sRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(sublog, w, sRes.Status)
		return
	}

	info := sRes.Info
	if info != nil {
		if info.Type != storageProvider.ResourceType_RESOURCE_TYPE_FILE {
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

	uReq := &storageProvider.InitiateFileUploadRequest{
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
		HandleErrorStatus(sublog, w, uRes.Status)
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

	ok, err := chunking.IsChunked(ref.GetId().GetOpaqueId())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if ok {
		chunk, err := chunking.GetChunkBLOBInfo(ref.GetId().GetOpaqueId())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		sReq = &storageProvider.StatRequest{
			Ref: &storageProvider.Reference{
				Spec: &storageProvider.Reference_Path{
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
		HandleErrorStatus(sublog, w, sRes.Status)
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
