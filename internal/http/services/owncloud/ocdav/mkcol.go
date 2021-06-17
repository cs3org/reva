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
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/rs/zerolog"
	"go.opencensus.io/trace"
)

func (s *svc) handlePathMkcol(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "mkcol")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()

	ref := &provider.Reference{Path: fn}

	s.handleMkcol(ctx, w, r, ref, sublog)
}

func (s *svc) handleSpacesMkCol(w http.ResponseWriter, r *http.Request, spaceID string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "spaces_mkcol")
	defer span.End()

	sublog := appctx.GetLogger(ctx).With().Str("path", r.URL.Path).Str("spaceid", spaceID).Str("handler", "mkcol").Logger()

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

	s.handleMkcol(ctx, w, r, ref, sublog)

}

func (s *svc) handleMkcol(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, log zerolog.Logger) {
	if r.Body != http.NoBody {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	gatewayClient, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check if ref exists
	statReq := &provider.StatRequest{Ref: ref}
	statRes, err := gatewayClient.Stat(ctx, statReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		if statRes.Status.Code == rpc.Code_CODE_OK {
			w.WriteHeader(http.StatusMethodNotAllowed) // 405 if it already exists
		} else {
			HandleErrorStatus(&log, w, statRes.Status)
		}
		return
	}

	req := &provider.CreateContainerRequest{Ref: ref}
	res, err := gatewayClient.CreateContainer(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending create container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	switch res.Status.Code {
	case rpc.Code_CODE_OK:
		w.WriteHeader(http.StatusCreated)
	case rpc.Code_CODE_NOT_FOUND:
		log.Debug().Str("path", r.URL.Path).Interface("status", statRes.Status).Msg("conflict")
		w.WriteHeader(http.StatusConflict)
	default:
		HandleErrorStatus(&log, w, res.Status)
	}
}
