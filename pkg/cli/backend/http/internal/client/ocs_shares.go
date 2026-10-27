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
	"strings"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// OCS addresses a share or a link by its id alone. The Graph routes need the
// item in the URL too.
const ocsShares = "/ocs/v2.php/apps/files_sharing/api/v1/shares"

const (
	OCSShareUser   = 0
	OCSShareGroup  = 1
	OCSSharePublic = 3
)

// OCSShare is a share or a public link as OCS reports it.
type OCSShare struct {
	ID           string `json:"id"`
	ShareType    int    `json:"share_type"`
	UIDOwner     string `json:"uid_owner"`
	UIDFileOwner string `json:"uid_file_owner"`
	Permissions  int    `json:"permissions"`
	STime        uint64 `json:"stime"`
	Expiration   string `json:"expiration"`
	Token        string `json:"token"`
	Name         string `json:"name"`
	ItemSource   string `json:"item_source"`
	ShareWith    string `json:"share_with"`
	Description  string `json:"description"`
}

func (c *Client) ocsShareURL(id string) string {
	return c.URL(ocsShares+"/"+url.PathEscape(id)) + "?format=json"
}

var ocsHeader = http.Header{"OCS-APIREQUEST": {"true"}, "Accept": {"application/json"}}

// GetShare returns the share or the link with the given id.
func (c *Client) GetShare(ctx context.Context, id string) (*OCSShare, error) {
	resp, err := c.do(ctx, request{method: http.MethodGet, url: c.ocsShareURL(id), header: ocsHeader, op: "get share", path: id})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env ocsEnvelope[[]OCSShare]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "get share", id, err)
	}
	if len(env.OCS.Data) == 0 {
		return nil, cberr.New(cberr.KindNotFound, "get share", id, "share not found")
	}
	return &env.OCS.Data[0], nil
}

// UpdateShare sets the form fields of a share or a link. An empty value is
// sent as it is, and removes a password.
func (c *Client) UpdateShare(ctx context.Context, id string, fields url.Values) (*OCSShare, error) {
	h := ocsHeader.Clone()
	h.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.do(ctx, request{method: http.MethodPut, url: c.ocsShareURL(id), header: h,
		body: stringBody(fields.Encode()), op: "update share", path: id})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env ocsEnvelope[OCSShare]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, cberr.Wrap(cberr.KindOther, "update share", id, err)
	}
	return &env.OCS.Data, nil
}

// RemoveShare removes a share or a link.
func (c *Client) RemoveShare(ctx context.Context, id string) error {
	resp, err := c.do(ctx, request{method: http.MethodDelete, url: c.ocsShareURL(id), header: ocsHeader, op: "remove share", path: id})
	if err != nil {
		return err
	}
	drain(resp)
	return nil
}

// GraphUser is a user that the Graph user search found.
type GraphUser struct {
	ID          string `json:"id"`
	Username    string `json:"onPremisesSamAccountName"`
	DisplayName string `json:"displayName"`
	Mail        string `json:"mail"`
}

// FindUsers searches the users by name or mail. The server wants at least
// three characters and an $orderby.
func (c *Client) FindUsers(ctx context.Context, query string) ([]GraphUser, error) {
	q := url.Values{"$search": {`"` + strings.ReplaceAll(query, `"`, "") + `"`}, "$orderby": {"displayName"}}
	var coll graphCollection[GraphUser]
	if err := c.getJSON(ctx, c.URL(graphV1+"/users")+"?"+q.Encode(), "find users", query, &coll); err != nil {
		return nil, err
	}
	return coll.Value, nil
}
