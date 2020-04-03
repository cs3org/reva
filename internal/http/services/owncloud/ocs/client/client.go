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

package client

import (
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

// MustGetGateway returns a client to the gateway service, returns an error otherwise
func MustGetGateway(addr string, r *http.Request, w http.ResponseWriter) gateway.GatewayAPIClient {
	client, err := pool.GetGatewayServiceClient(addr)
	if err != nil {
		response.WriteOCSError(w, r, response.MetaBadRequest.StatusCode, "no connection to gateway service", nil)
		return nil
	}

	return client
}
