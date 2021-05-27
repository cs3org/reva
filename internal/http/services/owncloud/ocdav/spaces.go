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
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageProvider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/grpc/services/storageprovider"
	"github.com/cs3org/reva/internal/http/services/datagateway"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/router"
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
		case http.MethodOptions:
			s.handleOptions(w, r, spaceID)
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

func (s *svc) handleSpacesPropfind(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "propfind")
	defer span.End()

	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "1"
	}

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Logger()

	// see https://tools.ietf.org/html/rfc4918#section-9.1
	if depth != "0" && depth != "1" && depth != "infinity" {
		sublog.Debug().Str("depth", depth).Msgf("invalid Depth header value")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	pf, status, err := readPropfind(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading propfind request")
		w.WriteHeader(status)
		return
	}

	// Get the getway client
	gatewayClient, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	metadataKeys := []string{}
	if pf.Allprop != nil {
		// TODO this changes the behavior and returns all properties if allprops has been set,
		// but allprops should only return some default properties
		// see https://tools.ietf.org/html/rfc4918#section-9.1
		// the description of arbitrary_metadata_keys in https://cs3org.github.io/cs3apis/#cs3.storage.provider.v1beta1.ListContainerRequest an others may need clarification
		// tracked in https://github.com/cs3org/cs3apis/issues/104
		metadataKeys = append(metadataKeys, "*")
	} else {
		for i := range pf.Prop {
			if requiresExplicitFetching(&pf.Prop[i]) {
				metadataKeys = append(metadataKeys, metadataKeyOf(&pf.Prop[i]))
			}
		}
	}

	// retrieve a specific storage space
	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	req := &storageProvider.StatRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: metadataKeys,
	}
	res, err := gatewayClient.Stat(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Interface("req", req).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}

	parentInfo := res.Info
	resourceInfos := []*storageProvider.ResourceInfo{parentInfo}
	if parentInfo.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER && depth == "1" {
		req := &storageProvider.ListContainerRequest{
			Ref:                   ref,
			ArbitraryMetadataKeys: metadataKeys,
		}
		res, err := gatewayClient.ListContainer(ctx, req)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending list container grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(&sublog, w, res.Status)
			return
		}
		resourceInfos = append(resourceInfos, res.Infos...)
	} else if depth == "infinity" {
		// FIXME: doesn't work cross-storage as the results will have the wrong paths!
		// use a stack to explore sub-containers breadth-first
		stack := []string{parentInfo.Path}
		for len(stack) > 0 {
			// retrieve path on top of stack
			currentPath := stack[len(stack)-1]
			ref = &storageProvider.Reference{
				Spec: &storageProvider.Reference_Path{Path: currentPath},
			}
			req := &storageProvider.ListContainerRequest{
				Ref:                   ref,
				ArbitraryMetadataKeys: metadataKeys,
			}
			res, err := gatewayClient.ListContainer(ctx, req)
			if err != nil {
				sublog.Error().Err(err).Str("path", currentPath).Msg("error sending list container grpc request")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if res.Status.Code != rpc.Code_CODE_OK {
				HandleErrorStatus(&sublog, w, res.Status)
				return
			}

			resourceInfos = append(resourceInfos, res.Infos...)

			if depth != "infinity" {
				break
			}

			// TODO: stream response to avoid storing too many results in memory

			stack = stack[:len(stack)-1]

			// check sub-containers in reverse order and add them to the stack
			// the reversed order here will produce a more logical sorting of results
			for i := len(res.Infos) - 1; i >= 0; i-- {
				// for i := range res.Infos {
				if res.Infos[i].Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
					stack = append(stack, res.Infos[i].Path)
				}
			}
		}
	}

	// prefix space id to paths
	for i := range resourceInfos {
		resourceInfos[i].Path = path.Join("/", spaceID, resourceInfos[i].Path)
	}

	propRes, err := s.formatPropfind(ctx, &pf, resourceInfos, "") // no namespace because this is relative to the storage space
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting propfind")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	var disableTus bool
	// let clients know this collection supports tus.io POST requests to start uploads
	if parentInfo.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		if parentInfo.Opaque != nil {
			_, disableTus = parentInfo.Opaque.Map["disable_tus"]
		}
		if !disableTus {
			w.Header().Add("Access-Control-Expose-Headers", "Tus-Resumable, Tus-Version, Tus-Extension")
			w.Header().Set("Tus-Resumable", "1.0.0")
			w.Header().Set("Tus-Version", "1.0.0")
			w.Header().Set("Tus-Extension", "creation,creation-with-upload")
		}
	}
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}

func (s *svc) handleSpacesMkCol(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "mkcol")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "mkcol").Logger()

	if r.Body != http.NoBody {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	gatewayClient, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	statReq := &storageProvider.StatRequest{Ref: ref}
	statRes, err := gatewayClient.Stat(ctx, statReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		if statRes.Status.Code == rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusMethodNotAllowed) // 405 if it already exists
		} else {
			HandleErrorStatus(&sublog, w, statRes.Status)
		}
		return
	}

	req := &storageProvider.CreateContainerRequest{Ref: ref}
	res, err := gatewayClient.CreateContainer(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending create container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusCreated)
	case rpc.Code_CODE_NOT_FOUND:
		sublog.Debug().Str("path", r.URL.Path).Interface("status", statRes.Status).Msg("conflict")
		w.WriteHeader(http.StatusConflict)
	default:
		HandleErrorStatus(&sublog, w, res.Status)
	}
}

func (s *svc) handleSpacesGet(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "get")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "get").Logger()

	// retrieve a specific storage space
	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	gatewayClient, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// TODO remove this stat. And error should also be returned by InitiateFileDownload
	sReq := &storageProvider.StatRequest{
		Ref: ref,
	}
	sRes, err := gatewayClient.Stat(ctx, sReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, sRes.Status)
		return
	}

	info := sRes.Info
	if info.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		sublog.Warn().Msg("resource is a folder and cannot be downloaded")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	dReq := &storageProvider.InitiateFileDownloadRequest{
		Ref: ref,
	}

	dRes, err := gatewayClient.InitiateFileDownload(ctx, dReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error initiating file download")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, dRes.Status)
		return
	}

	var ep, token string
	for _, p := range dRes.Protocols {
		if p.Protocol == "spaces" {
			ep, token = p.DownloadEndpoint, p.Token
		}
	}

	httpReq, err := rhttp.NewRequest(ctx, "GET", ep, nil)
	if err != nil {
		sublog.Error().Err(err).Msg("error creating http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set(datagateway.TokenTransportHeader, token)

	if r.Header.Get("Range") != "" {
		httpReq.Header.Set("Range", r.Header.Get("Range"))
	}

	httpClient := s.client

	httpRes, err := httpClient.Do(httpReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error performing http request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer httpRes.Body.Close()

	if httpRes.StatusCode != http.StatusOK && httpRes.StatusCode != http.StatusPartialContent {
		w.WriteHeader(httpRes.StatusCode)
		return
	}

	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+
		path.Base(info.Path)+"; filename=\""+path.Base(info.Path)+"\"")
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id)) // TODO why does the client need this
	w.Header().Set("OC-ETag", info.Etag)
	t := utils.TSToTime(info.Mtime).UTC()
	lastModifiedString := t.Format(time.RFC1123Z)
	w.Header().Set("Last-Modified", lastModifiedString)

	if httpRes.StatusCode == http.StatusPartialContent {
		w.Header().Set("Content-Range", httpRes.Header.Get("Content-Range"))
		w.Header().Set("Content-Length", httpRes.Header.Get("Content-Length"))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatUint(info.Size, 10))
	}
	if info.Checksum != nil {
		w.Header().Set("OC-Checksum", fmt.Sprintf("%s:%s", strings.ToUpper(string(storageprovider.GRPC2PKGXS(info.Checksum.Type))), info.Checksum.Sum))
	}
	var c int64
	if c, err = io.Copy(w, httpRes.Body); err != nil {
		sublog.Error().Err(err).Msg("error finishing copying data to response")
	}
	if httpRes.Header.Get("Content-Length") != "" {
		i, err := strconv.ParseInt(httpRes.Header.Get("Content-Length"), 10, 64)
		if err != nil {
			sublog.Error().Err(err).Str("content-length", httpRes.Header.Get("Content-Length")).Msg("invalid content length in datagateway response")
		}
		if i != c {
			sublog.Error().Int64("content-length", i).Int64("transferred-bytes", c).Msg("content length vs transferred bytes mismatch")
		}
	}
	// TODO we need to send the If-Match etag in the GET to the datagateway to prevent race conditions between stating and reading the file
}

func (s *svc) handleSpacesDelete(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Logger()
	// retrieve a specific storage space
	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &storageProvider.DeleteRequest{Ref: ref}
	res, err := client.Delete(ctx, req)
	if err != nil {
		sublog.Error().Err(err).Msg("error performing delete grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, res.Status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *svc) handleSpacesMove(w http.ResponseWriter, r *http.Request, srcSpaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "move")
	defer span.End()

	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")

	dst, err := extractDestination(dstHeader, r.Context().Value(ctxKeyBaseURI).(string))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sublog := appctx.GetLogger(ctx)
	sublog.Debug().Str("overwrite", overwrite).Msg("move")

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// retrieve a specific storage space
	srcRef, status, err := s.lookUpStorageSpaceReference(ctx, srcSpaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(sublog, w, status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check src exists
	srcStatReq := &storageProvider.StatRequest{Ref: srcRef}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(sublog, w, srcStatRes.Status)
		return
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	// retrieve a specific storage space
	dstRef, status, err := s.lookUpStorageSpaceReference(ctx, dstSpaceID, dstRelPath)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(sublog, w, status)
		return
	}
	dstStatReq := &storageProvider.StatRequest{Ref: dstRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(sublog, w, srcStatRes.Status)
		return
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4

	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if overwrite == "F" {
			sublog.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}

		// delete existing tree
		delReq := &storageProvider.DeleteRequest{Ref: dstRef}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc delete request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			HandleErrorStatus(sublog, w, delRes.Status)
			return
		}
	} else {
		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dstRelPath)
		// retrieve a specific storage space
		dstRef, status, err := s.lookUpStorageSpaceReference(ctx, dstSpaceID, intermediateDir)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending a grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(sublog, w, status)
			return
		}
		intStatReq := &storageProvider.StatRequest{Ref: dstRef}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				sublog.Debug().Str("parent", intermediateDir).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				HandleErrorStatus(sublog, w, intStatRes.Status)
			}
			return
		}
		// TODO what if intermediate is a file?
	}

	mReq := &storageProvider.MoveRequest{Source: srcRef, Destination: dstRef}
	mRes, err := client.Move(ctx, mReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending move grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if mRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(sublog, w, mRes.Status)
		return
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dstStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(sublog, w, dstStatRes.Status)
		return
	}

	info := dstStatRes.Info
	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id))
	w.Header().Set("OC-ETag", info.Etag)
	w.WriteHeader(successCode)
}

func (s *svc) handleSpacesProppatch(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "proppatch")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Logger()

	pp, status, err := readProppatch(r.Body)
	if err != nil {
		sublog.Debug().Err(err).Msg("error reading proppatch")
		w.WriteHeader(status)
		return
	}

	c, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// retrieve a specific storage space
	ref, rpcStatus, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}
	// check if resource exists
	statReq := &storageProvider.StatRequest{
		Ref: ref,
	}
	statRes, err := c.Stat(ctx, statReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, statRes.Status)
		return
	}

	rreq := &storageProvider.UnsetArbitraryMetadataRequest{
		Ref:                   ref,
		ArbitraryMetadataKeys: []string{""},
	}
	sreq := &storageProvider.SetArbitraryMetadataRequest{
		Ref: ref,
		ArbitraryMetadata: &storageProvider.ArbitraryMetadata{
			Metadata: map[string]string{},
		},
	}
	acceptedProps := []xml.Name{}
	removedProps := []xml.Name{}
	for i := range pp {
		if len(pp[i].Props) == 0 {
			continue
		}
		for j := range pp[i].Props {
			propNameXML := pp[i].Props[j].XMLName
			// don't use path.Join. It removes the double slash! concatenate with a /
			key := fmt.Sprintf("%s/%s", pp[i].Props[j].XMLName.Space, pp[i].Props[j].XMLName.Local)
			value := string(pp[i].Props[j].InnerXML)
			remove := pp[i].Remove
			// boolean flags may be "set" to false as well
			if s.isBooleanProperty(key) {
				// Make boolean properties either "0" or "1"
				value = s.as0or1(value)
				if value == "0" {
					remove = true
				}
			}
			// Webdav spec requires the operations to be executed in the order
			// specified in the PROPPATCH request
			// http://www.webdav.org/specs/rfc2518.html#rfc.section.8.2
			// FIXME: batch this somehow
			if remove {
				rreq.ArbitraryMetadataKeys[0] = key
				res, err := c.UnsetArbitraryMetadata(ctx, rreq)
				if err != nil {
					sublog.Error().Err(err).Msg("error sending a grpc UnsetArbitraryMetadata request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if res.Status.Code != rpc.Code_CODE_OK {
					HandleErrorStatus(&sublog, w, res.Status)
					return
				}
				removedProps = append(removedProps, propNameXML)
			} else {
				sreq.ArbitraryMetadata.Metadata[key] = value
				res, err := c.SetArbitraryMetadata(ctx, sreq)
				if err != nil {
					sublog.Error().Err(err).Str("key", key).Str("value", value).Msg("error sending a grpc SetArbitraryMetadata request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				if res.Status.Code != rpc.Code_CODE_OK {
					HandleErrorStatus(&sublog, w, res.Status)
					return
				}

				acceptedProps = append(acceptedProps, propNameXML)
				delete(sreq.ArbitraryMetadata.Metadata, key)
			}
		}
		// FIXME: in case of error, need to set all properties back to the original state,
		// and return the error in the matching propstat block, if applicable
		// http://www.webdav.org/specs/rfc2518.html#rfc.section.8.2
	}

	// nRef := strings.TrimPrefix(fn, ns)
	nRef := path.Join(spaceID, statRes.Info.Path)
	nRef = path.Join(ctx.Value(ctxKeyBaseURI).(string), nRef)
	if statRes.Info.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		nRef += "/"
	}

	propRes, err := s.formatProppatchResponse(ctx, acceptedProps, removedProps, nRef)
	if err != nil {
		sublog.Error().Err(err).Msg("error formatting proppatch response")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("DAV", "1, 3, extended-mkcol")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	if _, err := w.Write([]byte(propRes)); err != nil {
		sublog.Err(err).Msg("error writing response")
	}
}

func (s *svc) handleSpacesCopy(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "head")
	defer span.End()

	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")
	depth := r.Header.Get("Depth")
	if depth == "" {
		depth = "infinity"
	}

	dst, err := extractDestination(dstHeader, r.Context().Value(ctxKeyBaseURI).(string))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", spaceID).Str("path", r.URL.Path).Logger()
	sublog.Debug().Str("overwrite", overwrite).Str("depth", depth).Msg("copy")

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if depth != "infinity" && depth != "0" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// retrieve a specific storage space
	srcRef, status, err := s.lookUpStorageSpaceReference(ctx, spaceID, r.URL.Path)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}

	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	srcStatReq := &storageProvider.StatRequest{Ref: srcRef}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, srcStatRes.Status)
		return
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	// retrieve a specific storage space
	dstRef, status, err := s.lookUpStorageSpaceReference(ctx, dstSpaceID, dstRelPath)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, status)
		return
	}
	// check dst exists
	dstStatReq := &storageProvider.StatRequest{Ref: dstRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&sublog, w, srcStatRes.Status)
		return
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.8.5
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.8.5

		if overwrite == "F" {
			sublog.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return
		}

	} else {
		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dstRelPath)
		// retrieve a specific storage space
		intermediateRef, status, err := s.lookUpStorageSpaceReference(ctx, dstSpaceID, intermediateDir)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending a grpc request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if status.Code != rpc.Code_CODE_OK {
			HandleErrorStatus(&sublog, w, status)
			return
		}
		intStatReq := &storageProvider.StatRequest{Ref: intermediateRef}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				sublog.Debug().Str("parent", intermediateDir).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				HandleErrorStatus(&sublog, w, srcStatRes.Status)
			}
			return
		}
		// TODO what if intermediate is a file?
	}

	err = s.descendSpaces(ctx, client, srcStatRes.Info, dstRef, depth == "infinity")
	if err != nil {
		sublog.Error().Err(err).Str("depth", depth).Msg("error descending directory")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(successCode)
}
func (s *svc) descendSpaces(ctx context.Context, client gateway.GatewayAPIClient, src *storageProvider.ResourceInfo, dst *storageProvider.Reference, recurse bool) error {
	log := appctx.GetLogger(ctx)
	log.Debug().Str("src", src.Path).Interface("dst", dst).Msg("descending")
	if src.Type == storageProvider.ResourceType_RESOURCE_TYPE_CONTAINER {
		// create dir
		createReq := &storageProvider.CreateContainerRequest{
			Ref: dst,
		}
		createRes, err := client.CreateContainer(ctx, createReq)
		if err != nil || createRes.Status.Code != rpc.Code_CODE_OK {
			return err
		}

		// TODO: also copy properties: https://tools.ietf.org/html/rfc4918#section-9.8.2

		if !recurse {
			return nil
		}

		spaceID, _ := router.ShiftPath(dst.GetId().OpaqueId)

		// descend for children
		listReq := &storageProvider.ListContainerRequest{
			Ref: &storageProvider.Reference{
				Spec: &storageProvider.Reference_Id{
					Id: &storageProvider.ResourceId{
						StorageId: dst.GetId().StorageId,
						OpaqueId:  path.Join("/", spaceID, src.Path),
					}},
			},
		}
		res, err := client.ListContainer(ctx, listReq)
		if err != nil {
			return err
		}
		if res.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", res.Status.Code)
		}

		for i := range res.Infos {
			// childDst := path.Join(dst, path.Base(res.Infos[i].Path))
			childRef := &storageProvider.Reference{
				Spec: &storageProvider.Reference_Id{
					Id: &storageProvider.ResourceId{
						StorageId: dst.GetId().StorageId,
						OpaqueId:  path.Join(dst.GetId().OpaqueId, "..", res.Infos[i].Path),
					},
				},
			}
			err := s.descendSpaces(ctx, client, res.Infos[i], childRef, recurse)
			if err != nil {
				return err
			}
		}

	} else {
		// copy file

		// 1. get download url

		spaceID, _ := router.ShiftPath(dst.GetId().OpaqueId)
		dReq := &storageProvider.InitiateFileDownloadRequest{
			Ref: &storageProvider.Reference{
				Spec: &storageProvider.Reference_Id{
					Id: &storageProvider.ResourceId{
						StorageId: dst.GetId().StorageId,
						OpaqueId:  path.Join("/", spaceID, src.Path),
					},
				},
			},
		}

		dRes, err := client.InitiateFileDownload(ctx, dReq)
		if err != nil {
			return err
		}

		if dRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", dRes.Status.Code)
		}

		var downloadEP, downloadToken string
		for _, p := range dRes.Protocols {
			if p.Protocol == "spaces" {
				downloadEP, downloadToken = p.DownloadEndpoint, p.Token
			}
		}

		// 2. get upload url

		uReq := &storageProvider.InitiateFileUploadRequest{
			Ref: dst,
			Opaque: &typespb.Opaque{
				Map: map[string]*typespb.OpaqueEntry{
					"Upload-Length": {
						Decoder: "plain",
						// TODO: handle case where size is not known in advance
						Value: []byte(strconv.FormatUint(src.GetSize(), 10)),
					},
				},
			},
		}

		uRes, err := client.InitiateFileUpload(ctx, uReq)
		if err != nil {
			return err
		}

		if uRes.Status.Code != rpc.Code_CODE_OK {
			return fmt.Errorf("status code %d", uRes.Status.Code)
		}

		var uploadEP, uploadToken string
		for _, p := range uRes.Protocols {
			if p.Protocol == "simple" {
				uploadEP, uploadToken = p.UploadEndpoint, p.Token
			}
		}

		// 3. do download

		httpDownloadReq, err := rhttp.NewRequest(ctx, "GET", downloadEP, nil)
		if err != nil {
			return err
		}
		httpDownloadReq.Header.Set(datagateway.TokenTransportHeader, downloadToken)

		httpDownloadRes, err := s.client.Do(httpDownloadReq)
		if err != nil {
			return err
		}
		defer httpDownloadRes.Body.Close()
		if httpDownloadRes.StatusCode != http.StatusOK {
			return fmt.Errorf("status code %d", httpDownloadRes.StatusCode)
		}

		// 4. do upload

		if src.GetSize() > 0 {
			httpUploadReq, err := rhttp.NewRequest(ctx, "PUT", uploadEP, httpDownloadRes.Body)
			if err != nil {
				return err
			}
			httpUploadReq.Header.Set(datagateway.TokenTransportHeader, uploadToken)

			httpUploadRes, err := s.client.Do(httpUploadReq)
			if err != nil {
				return err
			}
			defer httpUploadRes.Body.Close()
			if httpUploadRes.StatusCode != http.StatusOK {
				return err
			}
		}
	}
	return nil
}
