// Copyright 2018-2026 CERN
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
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

const ocsClients = "/ocs/v1.php/cloud/user/clients"

// ConnectedClient is one app password.
type ConnectedClient struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	LastSeenAt  string `json:"last_seen_at,omitempty"`
}

type ocsEnvelope[T any] struct {
	OCS struct {
		Meta struct {
			Status     string `json:"status"`
			StatusCode int    `json:"statuscode"`
			Message    string `json:"message"`
		} `json:"meta"`
		Data T `json:"data"`
	} `json:"ocs"`
}

// ListConnectedClients returns the app passwords on the account.
func (c *Client) ListConnectedClients(ctx context.Context) ([]ConnectedClient, error) {
	resp, err := c.do(ctx, request{
		method: http.MethodGet,
		url:    c.URL(ocsClients) + "?format=json",
		header: http.Header{
			"OCS-APIREQUEST": []string{"true"},
			"Accept":         []string{"application/json"},
		},
		op: "list app tokens",
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var env ocsEnvelope[[]ConnectedClient]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "list app tokens", "", err)
	}
	if env.OCS.Meta.StatusCode != 0 && env.OCS.Meta.StatusCode != 100 && env.OCS.Meta.StatusCode != 200 {
		return nil, cberr.New(cberr.KindOther, "list app tokens", "", env.OCS.Meta.Message)
	}
	return env.OCS.Data, nil
}

// RevokeConnectedClient deletes an app password. Revoking an id that is already
// gone is not an error.
func (c *Client) RevokeConnectedClient(ctx context.Context, id string) error {
	resp, err := c.do(ctx, request{
		method: http.MethodDelete,
		url:    c.URL(ocsClients+"/"+url.PathEscape(id)) + "?format=json",
		header: http.Header{
			"OCS-APIREQUEST": []string{"true"},
			"Accept":         []string{"application/json"},
		},
		op:   "revoke app token",
		path: id,
	})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}
