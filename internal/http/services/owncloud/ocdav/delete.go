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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) handleDelete(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	ns = applyLayout(ctx, ns)

	fn := path.Join(ns, r.URL.Path)

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	req := &provider.DeleteRequest{Ref: ref}
	res, err := client.Delete(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error performing delete grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		log.Warn().Str("code", string(res.Status.Code)).Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		log.Warn().Str("code", string(res.Status.Code)).Msg("grpc request failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
