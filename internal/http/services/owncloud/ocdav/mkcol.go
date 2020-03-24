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
	"io"
	"net/http"
	"path"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
)

func (s *svc) handleMkcol(w http.ResponseWriter, r *http.Request, ns string) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := path.Join(ns, r.URL.Path)

	buf := make([]byte, 1)
	_, err := r.Body.Read(buf)
	if err != io.EOF {
		log.Error().Err(err).Msg("error reading request body")
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// check fn exists
	ref := &provider.Reference{
		Spec: &provider.Reference_Path{Path: fn},
	}
	statReq := &provider.StatRequest{Ref: ref}
	statRes, err := client.Stat(ctx, statReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code == rpc.Code_CODE_OK {
		log.Warn().Msg("resource already exists")
		w.WriteHeader(http.StatusMethodNotAllowed) // 405 if it already exists
		return
	}

	req := &provider.CreateContainerRequest{Ref: ref}
	res, err := client.CreateContainer(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending create container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		w.WriteHeader(http.StatusConflict)
		return
	}

	if res.Status.Code != rpc.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
