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
	"net/http"
	"path"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"go.opencensus.io/trace"
)

func (s *svc) handleMove(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "move")
	defer span.End()

	src := path.Join(ns, r.URL.Path)
	dstHeader := r.Header.Get("Destination")
	overwrite := r.Header.Get("Overwrite")

	dst, err := extractDestination(dstHeader, r.Context().Value(ctxKeyBaseURI).(string))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	dst = path.Join(ns, dst)

	sublog := appctx.GetLogger(ctx).With().Str("src", src).Str("dst", dst).Logger()
	sublog.Debug().Str("overwrite", overwrite).Msg("move")

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
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check src exists
	srcStatReq := &provider.StatRequest{
		Ref: &provider.Reference{
			Spec: &provider.Reference_Path{Path: src},
		},
	}
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

	// check dst exists
	dstStatRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	dstStatReq := &provider.StatRequest{Ref: dstStatRef}
	dstStatRes, err := client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if dstStatRes.Status.Code != rpc.Code_CODE_OK && dstStatRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
		HandleErrorStatus(&sublog, w, srcStatRes.Status)
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
		delReq := &provider.DeleteRequest{Ref: dstStatRef}
		delRes, err := client.Delete(ctx, delReq)
		if err != nil {
			sublog.Error().Err(err).Msg("error sending grpc delete request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if delRes.Status.Code != rpc.Code_CODE_OK && delRes.Status.Code != rpc.Code_CODE_NOT_FOUND {
			HandleErrorStatus(&sublog, w, delRes.Status)
			return
		}
	} else {
		// check if an intermediate path / the parent exists
		intermediateDir := path.Dir(dst)
		ref2 := &provider.Reference{
			Spec: &provider.Reference_Path{Path: intermediateDir},
		}
		intStatReq := &provider.StatRequest{Ref: ref2}
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
				HandleErrorStatus(&sublog, w, intStatRes.Status)
			}
			return
		}
		// TODO what if intermediate is a file?
	}

	sourceRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: src},
	}
	dstRef := &provider.Reference{
		Spec: &provider.Reference_Path{Path: dst},
	}
	mReq := &provider.MoveRequest{Source: sourceRef, Destination: dstRef}
	mRes, err := client.Move(ctx, mReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending move grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if mRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, mRes.Status)
		return
	}

	dstStatRes, err = client.Stat(ctx, dstStatReq)
	if err != nil {
		sublog.Error().Err(err).Msg("error sending grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if dstStatRes.Status.Code != rpc.Code_CODE_OK {
		HandleErrorStatus(&sublog, w, dstStatRes.Status)
		return
	}

	info := dstStatRes.Info
	w.Header().Set("Content-Type", info.MimeType)
	w.Header().Set("ETag", info.Etag)
	w.Header().Set("OC-FileId", wrapResourceID(info.Id))
	w.Header().Set("OC-ETag", info.Etag)
	w.WriteHeader(successCode)
}
