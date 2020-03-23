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

package ocmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

type share struct {
	ShareWith         string        `json:"shareWith"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	ProviderID        string        `json:"providerId"`
	Owner             string        `json:"owner"`
	Sender            string        `json:"sender"`
	OwnerDisplayName  string        `json:"ownerDisplayName"`
	SenderDisplayName string        `json:"senderDisplayName"`
	ShareType         string        `json:"shareType"`
	ResourceType      string        `json:"resourceType"`
	Protocol          *protocolInfo `json:"protocol"`

	ID        string `json:"id,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type protocolInfo struct {
	Name    string           `json:"name"`
	Options *protocolOptions `json:"options"`
}

type protocolOptions struct {
	SharedSecret string `json:"sharedSecret,omitempty"`
	Permissions  string `json:"permissions,omitempty"`
}

func (s *share) JSON() []byte {
	b, _ := json.MarshalIndent(s, "", "   ")
	return b

}

type sharesHandler struct {
	gatewayAddr string
}

func (h *sharesHandler) init(c *Config) error {
	h.gatewayAddr = c.GatewaySvc
	return nil
}

func (h *sharesHandler) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		log := appctx.GetLogger(r.Context())
		shareID := path.Base(r.URL.Path)
		log.Debug().Str("method", r.Method).Str("shareID", shareID).Msg("sharesHandler")

		switch r.Method {
		case http.MethodPost:
			h.createShare(w, r)
		case http.MethodGet:
			if shareID == "/" {
				h.listAllShares(w, r)
			} else {
				h.getShare(w, r, shareID)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (h *sharesHandler) createShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	sClient, err := pool.GetGatewayServiceClient(h.gatewayAddr)
	if err != nil {
		WriteOCSError(w, r, MetaServerError.StatusCode, "error getting storage grpc client", err)
		return
	}

}

func (h *sharesHandler) getShare(w http.ResponseWriter, r *http.Request, shareID string) {
}

func (h *sharesHandler) listAllShares(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(r.Context())
	user := r.Header.Get("Remote-User")

	log.Debug().Str("ctx", fmt.Sprintf("%+v", ctx)).Str("user", user).Msg("listAllShares")
	log.Debug().Str("Variable: `h` type", fmt.Sprintf("%T", h)).Str("Variable: `h` value", fmt.Sprintf("%+v", h)).Msg("listAllShares")

	shares, err := h.getShares(ctx, log, user)

	log.Debug().Str("err", fmt.Sprintf("%+v", err)).Str("shares", fmt.Sprintf("%+v", shares)).Msg("listAllShares")

	if err != nil {
		log.Err(err).Msg("Error reading shares from manager")
		w.WriteHeader(http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
