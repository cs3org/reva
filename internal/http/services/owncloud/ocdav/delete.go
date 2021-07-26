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
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/rs/zerolog"
	"go.opencensus.io/trace"
)

func (s *svc) handlePathDelete(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	ctx, span := trace.StartSpan(ctx, "delete")
	defer span.End()

	fn := path.Join(ns, r.URL.Path)

	sublog := appctx.GetLogger(ctx).With().Str("path", fn).Logger()
	ref := &provider.Reference{Path: fn}
	s.handleDelete(ctx, w, r, ref, sublog)
}

func (s *svc) handleDelete(ctx context.Context, w http.ResponseWriter, r *http.Request, ref *provider.Reference, log zerolog.Logger) {
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &provider.DeleteRequest{Ref: ref}
	res, err := client.Delete(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error performing delete grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if res.Status.Code != rpc.Code_CODE_OK {
		if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			m := fmt.Sprintf("Resource %v not found", ref.Path)
			b, err := Marshal(exception{
				code:    SabredavNotFound,
				message: m,
			})
			HandleWebdavError(&log, w, b, err)
		}
		if res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED {
			w.WriteHeader(http.StatusForbidden)
			m := fmt.Sprintf("Permission denied to delete %v", ref.Path)
			b, err := Marshal(exception{
				code:    SabredavPermissionDenied,
				message: m,
			})
			HandleWebdavError(&log, w, b, err)
		}
		if res.Status.Code == rpc.Code_CODE_INTERNAL && res.Status.Message == "can't delete mount path" {
			w.WriteHeader(http.StatusForbidden)
			b, err := Marshal(exception{
				code:    SabredavPermissionDenied,
				message: res.Status.Message,
			})
			HandleWebdavError(&log, w, b, err)
		}
		HandleErrorStatus(&log, w, res.Status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
