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
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/errors"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocdav/spacelookup"
	"github.com/cs3org/reva/pkg/appctx"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/rs/zerolog"
)

func (s *svc) handlePathMkcol(w http.ResponseWriter, r *http.Request, ns string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "mkcol")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)
	for _, r := range nameRules {
		if !r.Test(fn) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// stat requested path to make sure it isn't existing yet
	// NOTE: It could be on another storage provider than the 'parent' of it
	sr, err := client.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: fn,
		},
	})
	if err != nil {
		sublog.Error().Err(err).Str("path", fn).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if sr.Status.Code != rpc.Code_CODE_NOT_FOUND {
		sublog.Info().Err(err).Str("path", fn).Interface("code", sr.Status.Code).Msg("response code for stat was unexpected")
		// tests want this errorcode. StatusConflict would be more logical
		w.WriteHeader(http.StatusMethodNotAllowed)
		b, err := errors.Marshal(errors.SabredavMethodNotAllowed, "The resource you tried to create already exists", "")
		errors.HandleWebdavError(&sublog, w, b, err)
		return
	}

	parentPath := path.Dir(fn)

	space, status, err := spacelookup.LookUpStorageSpaceForPath(ctx, client, parentPath)
	if err != nil {
		sublog.Error().Err(err).Str("path", parentPath).Msg("failed to look up storage space")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handleMkcol(ctx, w, r, spacelookup.MakeRelativeReference(space, parentPath, false), spacelookup.MakeRelativeReference(space, fn, false), sublog)
}

func (s *svc) handleSpacesMkCol(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx, span := rtrace.Provider.Tracer("reva").Start(r.Context(), "spaces_mkcol")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "mkcol").Logger()
	client, err := s.getClient()
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	parentRef, rpcStatus, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, path.Dir(r.URL.Path), true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	childRef, rpcStatus, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, r.URL.Path, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	s.handleMkcol(ctx, w, r, parentRef, childRef, sublog)

}

func (s *svc) handleMkcol(ctx context.Context, w http.ResponseWriter, r *http.Request, parentRef, childRef *provider.Reference, log zerolog.Logger) {
	if r.Body != http.NoBody {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check if parent exists
	parentStatReq := &provider.StatRequest{Ref: parentRef}
	parentStatRes, err := client.Stat(ctx, parentStatReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if parentStatRes.Status.Code != rpc.Code_CODE_OK {
		if parentStatRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			// http://www.webdav.org/specs/rfc4918.html#METHOD_MKCOL
			// When the MKCOL operation creates a new collection resource,
			// all ancestors must already exist, or the method must fail
			// with a 409 (Conflict) status code.
			w.WriteHeader(http.StatusConflict)
			b, err := errors.Marshal(errors.SabredavNotFound, "Parent node does not exist", "")
			errors.HandleWebdavError(&log, w, b, err)
		} else {
			errors.HandleErrorStatus(&log, w, parentStatRes.Status)
		}
		return
	}

	// check if child exists
	statReq := &provider.StatRequest{Ref: childRef}
	statRes, err := client.Stat(ctx, statReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		if statRes.Status.Code == rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusMethodNotAllowed) // 405 if it already exists
			b, err := errors.Marshal(errors.SabredavMethodNotAllowed, "The resource you tried to create already exists", "")
			errors.HandleWebdavError(&log, w, b, err)
		} else {
			errors.HandleErrorStatus(&log, w, statRes.Status)
		}
		return
	}

	req := &provider.CreateContainerRequest{Ref: childRef}
	res, err := client.CreateContainer(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending create container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusCreated)
	case rpc.Code_CODE_NOT_FOUND:
		log.Debug().Str("path", childRef.Path).Interface("status", statRes.Status).Msg("conflict")
		w.WriteHeader(http.StatusConflict)
	case rpc.Code_CODE_PERMISSION_DENIED:
		w.WriteHeader(http.StatusForbidden)
		// TODO path could be empty or relative...
		m := fmt.Sprintf("Permission denied to create %v", childRef.Path)
		b, err := errors.Marshal(errors.SabredavPermissionDenied, m, "")
		errors.HandleWebdavError(&log, w, b, err)
	default:
		errors.HandleErrorStatus(&log, w, res.Status)
	}
}
