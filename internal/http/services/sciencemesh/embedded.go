// Copyright 2018-2025 CERN
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
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"

	"github.com/cs3org/reva/v3/internal/http/services/reqres"
	"github.com/cs3org/reva/v3/pkg/appctx"

	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
)

type embeddedHandler struct {
	gatewayClient gateway.GatewayAPIClient
}

func (h *embeddedHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}

	return nil
}

type ocmEmbeddedShareResponse struct {
	Id         string `json:"remote_share_id"`
	Name       string `json:"name"`
	ItemType   string `json:"item_type"`
	Owner      string `json:"owner"`
	ShareWith  string `json:"share_with"`
	Initiator  string `json:"initiator"`
	Expiration int64  `json:"expiration"`
	State      string `json:"state"`
	Alias      string `json:"alias"`
	Hidden     bool   `json:"hidden"`
}

// ListEmbeddedShares lists all embedded ocm shares for the current user.
func (h *embeddedHandler) ListEmbeddedShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	ocm_shares, err := h.gatewayClient.ListReceivedOCMShares(ctx, &ocm.ListReceivedOCMSharesRequest{
		Filters: []*ocm.ListReceivedOCMSharesRequest_Filter{
			{
				Type: ocm.ListReceivedOCMSharesRequest_Filter_TYPE_SHARE_TYPE,
				Term: &ocm.ListReceivedOCMSharesRequest_Filter_ResourceType{
					ResourceType: ocm.ShareType_SHARE_TYPE_EMBEDDED,
				},
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("error listing ocm embedded shares")
		reqres.WriteError(w, r, reqres.APIErrorServerError, "error listing ocm embedded shares", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
