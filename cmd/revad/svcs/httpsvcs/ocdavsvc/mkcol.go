// Copyright 2018-2019 CERN
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

package ocdavsvc

import (
	"io"
	"net/http"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cernbox/reva/pkg/appctx"
)

func (s *svc) doMkcol(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	fn := r.URL.Path

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
	ref := &storageproviderv0alphapb.Reference{
		Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
	}
	statReq := &storageproviderv0alphapb.StatRequest{Ref: ref}
	statRes, err := client.Stat(ctx, statReq)
	if err != nil {
		log.Error().Err(err).Msg("error sending a grpc stat request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if statRes.Status.Code == rpcpb.Code_CODE_OK {
		log.Warn().Msg("resource already exists")
		w.WriteHeader(http.StatusMethodNotAllowed) // 405 if it already exists
		return
	}

	req := &storageproviderv0alphapb.CreateContainerRequest{Ref: ref}
	res, err := client.CreateContainer(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending create container grpc request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
		w.WriteHeader(http.StatusConflict)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}
