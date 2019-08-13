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

package appregistrysvc

import (
	"encoding/json"
	"net/http"

	appregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/appregistry/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
)

func (s *svc) doList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	_, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &appregistryv0alphapb.ListAppProvidersRequest{}

	res, err := client.ListAppProviders(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc ListAppProviders request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if res.Status.Code != rpcpb.Code_CODE_OK {
		if res.Status.Code == rpcpb.Code_CODE_NOT_FOUND {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		log.Warn().Str("code", string(res.Status.Code)).Msg("grpc request failed")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var rawResponse = make([]string, len(res.Providers))
	for _, provider := range res.Providers {
		rawResponse = append(rawResponse, provider.Address)
	}
	finalResponse, _ := json.Marshal(rawResponse)

	_, err = w.Write(finalResponse)
	if err != nil {
		log.Warn().Str("code", string(res.Status.Code)).Msg("couldn't write to response")
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
