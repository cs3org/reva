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
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/net"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/router"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/resourceid"
	"github.com/rs/zerolog"
)

func (s *svc) handlePathMove(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "move")
	defer span.End()

	srcPath := path.Join(ns, r.URL.Path)
	dstPath, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	for _, r := range nameRules {
		if !r.Test(dstPath) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	dstPath = path.Join(ns, dstPath)

	sublog := appctx.GetLogger(ctx).With().Str("src", srcPath).Str("dst", dstPath).Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	srcSpace, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, srcPath)
	if err != nil {
		sublog.Error().Err(err).Str("path", srcPath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}
	dstSpace, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, dstPath)
	if err != nil {
		sublog.Error().Err(err).Str("path", srcPath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	// FIXME I suck
	if dstSpace.Root.OpaqueId == utils.ShareStorageProviderID {
		dstSpace.Root = srcSpace.Root
	}

	s.handleMove(ctx, w, r, spacelookup.MakeRelativeReference(srcSpace, srcPath, false), spacelookup.MakeRelativeReference(dstSpace, dstPath, false), sublog)
}

func (s *svc) handleSpacesMove(w http.ResponseWriter, r *http.Request, srcSpaceID string) {
	ctx, span := rtrace.Provider.Tracer("ocdav").Start(r.Context(), "spaces_move")
	defer span.End()

	dst, err := extractDestination(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sublog := appctx.GetLogger(ctx).With().Str("spaceid", srcSpaceID).Str("path", r.URL.Path).Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// retrieve a specific storage space
	srcRef, status, err := spacelookup.LookUpStorageSpaceReference(ctx, client, srcSpaceID, r.URL.Path, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	dstSpaceID, dstRelPath := router.ShiftPath(dst)

	// retrieve a specific storage space
	dstRef, status, err := spacelookup.LookUpStorageSpaceReference(ctx, client, dstSpaceID, dstRelPath, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handleMove(ctx, w, r, srcRef, dstRef, sublog)
}

func (s *svc) handleMove(ctx context.Context, w http.ResponseWriter, r *http.Request, src, dst *provider.Reference, log zerolog.Logger) {
	overwrite := r.Header.Get(net.HeaderOverwrite)
	log.Debug().Str("overwrite", overwrite).Msg("move")

	overwrite = strings.ToUpper(overwrite)
	if overwrite == "" {
		overwrite = "T"
	}

	if overwrite != "T" && overwrite != "F" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check src exists
	srcStatReq := &provider.StatRequest{Ref: src}
	srcStatRes, err := client.Stat(ctx, srcStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if srcStatRes.Status.Code != rpc.Code_CODE_OK {
		if srcStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			m := fmt.Sprintf("Resource %v not found", srcStatReq.Ref.Path)
			b, err := errors.Marshal(http.StatusNotFound, m, "")
			errors.HandleWebdavError(&log, w, b, err)
		}
		errors.HandleErrorStatus(&log, w, srcStatRes.Status)
		return
	}

	// check dst exists
	dstStatReq := &provider.StatRequest{Ref: dst}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		errors.HandleErrorStatus(&log, w, srcStatRes.Status)
		return
	}

	successCode := http.StatusCreated // 201 if new resource was created, see https://tools.ietf.org/html/rfc4918#section-9.9.4
	if dstStatRes.Status.Code == rpc.Code_CODE_OK {
		successCode = http.StatusNoContent // 204 if target already existed, see https://tools.ietf.org/html/rfc4918#section-9.9.4

		if overwrite == "F" {
			log.Warn().Str("overwrite", overwrite).Msg("dst already exists")
			w.WriteHeader(http.StatusPreconditionFailed) // 412, see https://tools.ietf.org/html/rfc4918#section-9.9.4
			return
		}

		// delete existing tree
		delReq := &provider.DeleteRequest{Ref: dst}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc delete request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			errors.HandleErrorStatus(&log, w, delRes.Status)
			return
		}
	} else {
		// check if an intermediate path / the parent exists
		intStatReq := &provider.StatRequest{Ref: &provider.Reference{
			ResourceId: dst.ResourceId,
			Path:       utils.MakeRelativePath(path.Dir(dst.Path)),
		}}
		intStatRes, err := client.Stat(ctx, intStatReq)
		if err != nil {
			log.Error().Err(err).Msg("error sending grpc stat request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if intStatRes.Status.Code != rpc.Code_CODE_OK {
			if intStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
				// 409 if intermediate dir is missing, see https://tools.ietf.org/html/rfc4918#section-9.8.5
				log.Debug().Interface("parent", dst).Interface("status", intStatRes.Status).Msg("conflict")
				w.WriteHeader(http.StatusConflict)
			} else {
				errors.HandleErrorStatus(&log, w, intStatRes.Status)
			}
			return
		}
		// TODO what if intermediate is a file?
	}

	mReq := &provider.MoveRequest{Source: src, Destination: dst}
	mRes, err := client.Move(ctx, mReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending move grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if mRes.Status.Code != rpc.Code_CODE_OK {
		if mRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			w.WriteHeader(http.StatusForbidden)
			m := fmt.Sprintf("Permission denied to move %v", src.Path)
			b, err := errors.Marshal(http.StatusForbidden, m, "")
			errors.HandleWebdavError(&log, w, b, err)
		}
		errors.HandleErrorStatus(&log, w, mRes.Status)
		return
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dstStatRes.Status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&log, w, dstStatRes.Status)
		return
	}

	info := dstStatRes.Info
	w.Header().Set(net.HeaderContentType, info.MimeType)
	w.Header().Set(net.HeaderETag, info.Etag)
	w.Header().Set(net.HeaderOCFileID, resourceid.OwnCloudResourceIDWrap(info.Id))
	w.Header().Set(net.HeaderOCETag, info.Etag)
	w.WriteHeader(successCode)
}
