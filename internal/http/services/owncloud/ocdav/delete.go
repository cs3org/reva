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

func (s *svc) handlePathDelete(w http.ResponseWriter, r *http.Request, ns string) {
	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(r.Context()).With().Str("path", fn).Logger()
	client, err := s.getClient()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	space, status, err := spacelookup.LookUpStorageSpaceForPath(r.Context(), client, fn)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if status.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, status)
		return
	}

	s.handleDelete(r.Context(), w, r, spacelookup.MakeRelativeReference(space, fn, false), sublog)
}

func (s *svc) handleDelete(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, log zerolog.Logger) {
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ctx, span := rtrace.Provider.Tracer("reva").Start(ctx, "delete")
	defer span.End()

	req := &provider.DeleteRequest{Ref: ref}
	res, err := client.Delete(ctx, req)
	if err != nil {
		span.RecordError(err)
		log.Error().Err(err).Msg("error performing delete grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			// TODO path might be empty or relative...
			m := fmt.Sprintf("Resource %v not found", ref.Path)
			b, err := errors.Marshal(errors.SabredavNotFound, m, "")
			errors.HandleWebdavError(&log, w, b, err)
		}
		if res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			w.WriteHeader(http.StatusForbidden)
			// TODO path might be empty or relative...
			m := fmt.Sprintf("Permission denied to delete %v", ref.Path)
			b, err := errors.Marshal(errors.SabredavPermissionDenied, m, "")
			errors.HandleWebdavError(&log, w, b, err)
		}
		if res.Status.Code == rpc.Code_CODE_INTERNAL && res.Status.Message == "can't delete mount path" {
			w.WriteHeader(http.StatusForbidden)
			b, err := errors.Marshal(errors.SabredavPermissionDenied, res.Status.Message, "")
			errors.HandleWebdavError(&log, w, b, err)
		}

		errors.HandleErrorStatus(&log, w, res.Status)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *svc) handleSpacesDelete(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := rtrace.Provider.Tracer("reva").Start(ctx, "spaces_delete")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Logger()
	client, err := s.getClient()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// retrieve a specific storage space
	ref, rpcStatus, err := spacelookup.LookUpStorageSpaceReference(ctx, client, spaceID, r.URL.Path, true)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending a grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rpcStatus.Code != rpc.Code_CODE_OK {
		errors.HandleErrorStatus(&sublog, w, rpcStatus)
		return
	}

	s.handleDelete(ctx, w, r, ref, sublog)
}
