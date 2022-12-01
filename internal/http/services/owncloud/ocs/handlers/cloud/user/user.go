// Copyright 2018-2022 CERN
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

package user

import (
	"context"
	"fmt"
	"net/http"

	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
)

// The Handler renders the user endpoint.
type Handler struct {
	gatewayAddr string
}

// Init initializes this and any contained handlers.
func (h *Handler) Init(c *config.Config) {
	h.gatewayAddr = c.GatewaySvc
}

const (
	defaultLanguage   = "en"
	languageNamespace = "core"
	languageKey       = "lang"
)

// GetSelf handles GET requests on /cloud/user.
func (h *Handler) GetSelf(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO move user to handler parameter?
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		response.WriteOCSError(w, r, response.MetaServerError.StatusCode, "missing user in context", fmt.Errorf("missing user in context"))
		return
	}

	response.WriteOCSSuccess(w, r, &User{
		ID:          u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Mail,
		UserType:    conversions.UserTypeString(u.Id.Type),
		Language:    h.getLanguage(ctx),
	})
}

func (h *Handler) getLanguage(ctx context.Context) string {
	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(h.gatewayAddr))
	if err != nil {
		return defaultLanguage
	}
	res, err := gw.GetKey(ctx, &preferences.GetKeyRequest{
		Key: &preferences.PreferenceKey{
			Namespace: languageNamespace,
			Key:       languageKey,
		},
	})
	if err != nil || res.Status.Code != rpc.Code_CODE_OK {
		return defaultLanguage
	}
	return res.GetVal()
}

// User holds user data.
type User struct {
	// TODO needs better naming, clarify if we need a userid, a username or both
	ID          string `json:"id" xml:"id"`
	DisplayName string `json:"display-name" xml:"display-name"`
	Email       string `json:"email" xml:"email"`
	UserType    string `json:"user-type" xml:"user-type"`
	Language    string `json:"language" xml:"language"`
}
