// Copyright 2018-2024 CERN
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

package sciencemesh

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

type providersHandler struct {
	gatewayClient gateway.GatewayAPIClient
}

func (h *providersHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}

	return nil
}

type provider struct {
	FullName string `json:"full_name"`
	Domain   string `json:"domain"`
}

// ListProviders lists all the providers filtering by the `search` query parameter.
func (h *providersHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	term := strings.ToLower(r.URL.Query().Get("search"))

	listRes, err := h.gatewayClient.ListAllProviders(ctx, &providerpb.ListAllProvidersRequest{})
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error listing all providers", err)
		return
	}

	if listRes.Status.Code != rpc.Code_CODE_OK {
		reqres.WriteError(w, r, reqres.APIErrorServerError, listRes.Status.Message, errors.New(listRes.Status.Message))
		return
	}

	filtered := []*provider{}
	for _, p := range listRes.Providers {
		if strings.Contains(strings.ToLower(p.FullName), term) ||
			strings.Contains(strings.ToLower(p.Domain), term) {
			filtered = append(filtered, &provider{
				FullName: p.FullName,
				Domain:   p.Domain,
			})
		}
	}

	if err := json.NewEncoder(w).Encode(filtered); err != nil {
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error encoding response in json", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
