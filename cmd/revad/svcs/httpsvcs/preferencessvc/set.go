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

package preferencessvc

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/user"
)

type parameter struct {
	Value string
}

func (s *svc) doSet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	_, ok := user.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var key string
	key, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error().Msg("error reading request body")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var p parameter
	err = json.Unmarshal(body, &p)
	if err != nil {
		log.Error().Msg("error getting value from request")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	req := &preferencesv0alphapb.SetKeyRequest{
		Key: key,
		Val: p.Value,
	}

	res, err := client.SetKey(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("error sending grpc SetKey request")
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

	w.WriteHeader(http.StatusOK)
}
