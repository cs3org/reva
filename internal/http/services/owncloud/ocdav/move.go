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
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/v2/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/v2/pkg/appctx"
	"github.com/cs3org/reva/v2/pkg/errtypes"
	rstatus "github.com/cs3org/reva/v2/pkg/rgrpc/status"
	"github.com/cs3org/reva/v2/pkg/rhttp/router"
	"github.com/cs3org/reva/v2/pkg/storagespace"
	"github.com/cs3org/reva/v2/pkg/utils"
)

func (s *svc) handlePathMove(w http.ResponseWriter, r *http.Request, ns string) (status int, err error) {
	ctx, span := s.tracerProvider.Tracer(tracerName).Start(r.Context(), "move")
	defer span.End()

	if r.Body != http.NoBody {
		return http.StatusUnsupportedMediaType, nil
	}

	srcPath := path.Join(ns, r.URL.Path)
	dh := r.Header.Get(net.HeaderDestination)
	baseURI := r.Context().Value(net.CtxKeyBaseURI).(string)
	dstPath, err := net.ParseDestination(baseURI, dh)
	if err != nil {
		return http.StatusBadRequest, err
	}

	for _, r := range nameRules {
		if !r.Test(srcPath) {
			w.WriteHeader(http.StatusBadRequest)
			b, err := errors.Marshal(http.StatusBadRequest, "source failed naming rules", "")
			errors.HandleWebdavError(appctx.GetLogger(ctx), w, b, err)
			return
		}
		if !r.Test(dstPath) {
			return http.StatusBadRequest, fmt.Errorf("path violates naming rules")
		}
	}

	dstPath = path.Join(ns, dstPath)

	client, err := s.getClient()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	srcSpace, rpcStatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, srcPath)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case rpcStatus.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(rpcStatus.Code), errtypes.NewErrtypeFromStatus(rpcStatus)
	}
	dstSpace, rpcStatus, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, dstPath)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case rpcStatus.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(rpcStatus.Code), errtypes.NewErrtypeFromStatus(rpcStatus)
	}

	return s.handleMove(ctx, w, r, spacelookup.MakeRelativeReference(srcSpace, srcPath, false), spacelookup.MakeRelativeReference(dstSpace, dstPath, false))
}

func (s *svc) handleSpacesMove(w http.ResponseWriter, r *http.Request, srcSpaceID string) (status int, err error) {
	ctx, span := s.tracerProvider.Tracer(tracerName).Start(r.Context(), "spaces_move")
	defer span.End()

	if r.Body != http.NoBody {
		return http.StatusUnsupportedMediaType, nil
	}

	dh := r.Header.Get(net.HeaderDestination)
	baseURI := r.Context().Value(net.CtxKeyBaseURI).(string)
	dst, err := net.ParseDestination(baseURI, dh)
	if err != nil {
		return http.StatusBadRequest, nil
	}

	srcRef, err := spacelookup.MakeStorageSpaceReference(srcSpaceID, r.URL.Path)
	if err != nil {
		return http.StatusBadRequest, nil
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	dstRef, err := spacelookup.MakeStorageSpaceReference(dstSpaceID, dstRelPath)
	if err != nil {
		return http.StatusBadRequest, nil
	}

	return s.handleMove(ctx, w, r, &srcRef, &dstRef)
}

func (s *svc) handleMove(ctx context.Context, w http.ResponseWriter, r *http.Request, src, dst *provider.Reference) (status int, err error) {
	ctx, span := s.tracerProvider.Tracer(tracerName).Start(ctx, "move")
	defer span.End()

	client, err := s.getClient()
	if err != nil {
		span.RecordError(err)
		return http.StatusInternalServerError, err
	}

	isChild, err := s.referenceIsChildOf(ctx, client, dst, src)
	if err != nil {
		switch err.(type) {
		case errtypes.IsNotSupported:
			return http.StatusForbidden, fmt.Errorf("can not detect recursive move operation. missing machine auth configuration?")
		default:
			span.RecordError(err)
			return http.StatusInternalServerError, err
		}
	}
	if isChild {
		return http.StatusConflict, fmt.Errorf("can not move a folder into one of its children")
	}

	oh := r.Header.Get(net.HeaderOverwrite)

	overwrite, err := net.ParseOverwrite(oh)
	if err != nil {
		return http.StatusBadRequest, nil
	}

	// check src exists
	srcStatReq := &provider.StatRequest{Ref: src}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case srcStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return http.StatusNotFound, fmt.Errorf("resource %v not found", srcStatReq.Ref.Path)
	case srcStatRes.Status.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(srcStatRes.Status.Code), errtypes.NewErrtypeFromStatus(srcStatRes.Status)
	}

	// check dst exists
	dstStatReq := &provider.StatRequest{Ref: dst}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND:
		return rstatus.HTTPStatusFromCode(dstStatRes.Status.Code), errtypes.NewErrtypeFromStatus(dstStatRes.Status)
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if !overwrite {
			// 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return http.StatusPreconditionFailed, fmt.Errorf("destination already exists")
		}

		// delete existing tree
		delReq := &provider.DeleteRequest{Ref: dst}
		delRes, err := client.Delete(ctx, delReq)
		switch {
		case err != nil:
			span.RecordError(err)
			return http.StatusInternalServerError, err
		case delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND:
			return rstatus.HTTPStatusFromCode(delRes.Status.Code), errtypes.NewErrtypeFromStatus(delRes.Status)
		}
	} else {
		// check if an intermediate path / the parent exists
		intStatReq := &provider.StatRequest{Ref: &provider.Reference{
			ResourceId: dst.ResourceId,
			Path:       utils.MakeRelativePath(path.Dir(dst.Path)),
		}}
		intStatRes, err := client.Stat(ctx, intStatReq)
		switch {
		case err != nil:
			span.RecordError(err)
			return http.StatusInternalServerError, err
		case intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
			// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
			return http.StatusConflict, fmt.Errorf(intStatRes.Status.Message)
		case intStatRes.Status.Code != rpc.Code_CODE_OK:
			return rstatus.HTTPStatusFromCode(intStatRes.Status.Code), errtypes.NewErrtypeFromStatus(intStatRes.Status)
		}
		// TODO what if intermediate is a file?
	}

	mReq := &provider.MoveRequest{Source: src, Destination: dst}
	mRes, err := client.Move(ctx, mReq)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case mRes.Status.Code == rpc.Code_CODE_ABORTED:
		return http.StatusPreconditionFailed, errtypes.NewErrtypeFromStatus(mRes.Status)
	case mRes.Status.Code == rpc.Code_CODE_UNIMPLEMENTED:
		// We translate this into a Bad Gateway error as per https://www.rfc-editor.org/rfc/rfc4918#section-9.9.4
		// > 502 (Bad Gateway) - This may occur when the destination is on another
		// > server and the destination server refuses to accept the resource.
		// > This could also occur when the destination is on another sub-section
		// > of the same server namespace.
		return http.StatusBadGateway, errtypes.NewErrtypeFromStatus(mRes.Status)
	case mRes.Status.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(mRes.Status.Code), errtypes.NewErrtypeFromStatus(mRes.Status)
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	switch {
	case err != nil:
		span.RecordError(err)
		return http.StatusInternalServerError, err
	case dstStatRes.Status.Code != rpc.Code_CODE_OK:
		return rstatus.HTTPStatusFromCode(dstStatRes.Status.Code), errtypes.NewErrtypeFromStatus(dstStatRes.Status)
	}

	info := dstStatRes.Info
	w.Header().Set(net.HeaderContentType, info.MimeType)
	w.Header().Set(net.HeaderETag, info.Etag)
	w.Header().Set(net.HeaderOCFileID, storagespace.FormatResourceID(*info.Id))
	w.Header().Set(net.HeaderOCETag, info.Etag)
	return successCode, nil
}
