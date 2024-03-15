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

package ocmd

import (
	"io"
	"mime"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"

	"github.com/cs3org/reva/internal/http/services/reqres"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

// var validate = validator.New()

type notifHandler struct {
	gatewayClient gateway.GatewayAPIClient
}

func (h *notifHandler) init(c *config) error {
	var err error
	h.gatewayClient, err = pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySvc))
	if err != nil {
		return err
	}
	return nil
}

// type notificationRequest struct {
//	NotificationType string `json:"notificationType" validate:"required"`
//	ResourceType     string `json:"resourceType" validate:"required"`
//	ProviderId       string `json:"providerId" validate:"required"`
//	Notification 	 ...	`json:"notification"`
//}

// Example of payload from Nextcloud:
// {
//   "notificationType": <one of "SHARE_ACCEPTED", "SHARE_DECLINED", "REQUEST_RESHARE", "SHARE_UNSHARED", "RESHARE_UNDO", "RESHARE_CHANGE_PERMISSION">,
//   "resourceType" : "file",
//   "providerId" : <shareId>,
//   "notification" : {
//  	"sharedSecret" : <token>,
//  	"message" : "human-readable message",
//  	"shareWith" : <user>,
// 	"senderId" : <user>,
//  	"shareType" : <type>
//   }
// }

// Notifications dispatches any notifications received from remote OCM sites
// according to the specifications at:
// https://cs3org.github.io/OCM-API/docs.html?branch=v1.1.0&repo=OCM-API&user=cs3org#/paths/~1notifications/post
func (h *notifHandler) Notifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := appctx.GetLogger(ctx)
	req, err := getNotification(r)
	if err != nil {
		reqres.WriteError(w, r, reqres.APIErrorInvalidParameter, err.Error(), nil)
		return
	}

	// TODO(lopresti) this is all to be implemented. For now we just log what we got
	log.Debug().Msgf("Received OCM notification: %+v", req)

	// this is to please Nextcloud
	w.WriteHeader(http.StatusCreated)
}

func getNotification(r *http.Request) (string, error) { // (*notificationRequest, error)
	// var req notificationRequest
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && contentType == "application/json" {
		bytes, _ := io.ReadAll(r.Body)
		return string(bytes), nil
		// if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		//	return nil, err
		//}
		// } else {
		//	return nil, errors.New("body request not recognised")
	}
	return "", nil
	// validate the request
	// if err := validate.Struct(req); err != nil {
	//	return nil, err
	//}
	// return &req, nil
}
