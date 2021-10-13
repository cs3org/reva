// Copyright 2018-2021 CERN
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

// Package nextcloud verifies a clientID and clientSecret against a Nextcloud backend.
package nextcloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

func init() {
	registry.Register("nextcloud", New)
}

// Manager is the Nextcloud-based implementation of the share.Manager interface
// see https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
type Manager struct {
	client   *http.Client
	endPoint string
}

// ShareManagerConfig contains config for a Nextcloud-based ShareManager
type ShareManagerConfig struct {
	EndPoint string `mapstructure:"endpoint" docs:";The Nextcloud backend endpoint for user check"`
	MockHTTP bool   `mapstructure:"mock_http"`
}

// Action describes a REST request to forward to the Nextcloud backend
type Action struct {
	verb string
	argS string
}

// GranteeAltMap is an alternative map to JSON-unmarshal a Grantee
// Grantees are hard to unmarshal, so unmarshalling into a map[string]interface{} first,
// see also https://github.com/pondersource/sciencemesh-nextcloud/issues/27
type GranteeAltMap struct {
	ID *provider.Grantee_UserId `json:"id"`
}

// ShareAltMap is an alternative map to JSON-unmarshal a Share
type ShareAltMap struct {
	ID          *ocm.ShareId          `json:"id"`
	ResourceID  *provider.ResourceId  `json:"resource_id"`
	Permissions *ocm.SharePermissions `json:"permissions"`
	Grantee     *GranteeAltMap        `json:"grantee"`
	Owner       *userpb.UserId        `json:"owner"`
	Creator     *userpb.UserId        `json:"creator"`
	Ctime       *types.Timestamp      `json:"ctime"`
	Mtime       *types.Timestamp      `json:"mtime"`
}

// ReceivedShareAltMap is an alternative map to JSON-unmarshal a ReceivedShare
type ReceivedShareAltMap struct {
	Share *ShareAltMap   `json:"share"`
	State ocm.ShareState `json:"state"`
}

func (c *ShareManagerConfig) init() {
}

func parseConfig(m map[string]interface{}) (*ShareManagerConfig, error) {
	c := &ShareManagerConfig{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

func getUser(ctx context.Context) (*userpb.User, error) {
	u, ok := ctxpkg.ContextGetUser(ctx)
	if !ok {
		err := errors.Wrap(errtypes.UserRequired(""), "nextcloud storage driver: error getting user from ctx")
		return nil, err
	}
	return u, nil
}

// New returns a share manager implementation that verifies against a Nextcloud backend.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	return NewShareManager(c)
}

// NewShareManager returns a new Nextcloud-based ShareManager
func NewShareManager(c *ShareManagerConfig) (*Manager, error) {
	var client *http.Client
	if c.MockHTTP {
		// called := make([]string, 0)
		// nextcloudServerMock := GetNextcloudServerMock(&called)
		// client, _ = TestingHTTPClient(nextcloudServerMock)

		// Wait for SetHTTPClient to be called later
		client = nil
	} else {
		client = &http.Client{}
	}

	return &Manager{
		endPoint: c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		client:   client,
	}, nil
}

// SetHTTPClient sets the HTTP client
func (sm *Manager) SetHTTPClient(c *http.Client) {
	sm.client = c
}

func (sm *Manager) do(ctx context.Context, a Action) (int, []byte, error) {
	log := appctx.GetLogger(ctx)
	user, err := getUser(ctx)
	if err != nil {
		return 0, nil, err
	}
	// url := am.endPoint + "~" + a.username + "/api/" + a.verb
	// url := "http://localhost/apps/sciencemesh/~" + user.Username + "/api/share/" + a.verb
	url := sm.endPoint + "~" + user.Username + "/api/ocm/" + a.verb

	log.Info().Msgf("am.do %s %s", url, a.argS)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := sm.client.Do(req)
	if err != nil {
		return 0, nil, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}

	log.Info().Msgf("am.do response %d %s", resp.StatusCode, body)
	return resp.StatusCode, body, nil
}

// Share as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) Share(ctx context.Context, md *provider.ResourceId, g *ocm.ShareGrant, name string,
	pi *ocmprovider.ProviderInfo, pm string, owner *userpb.UserId, token string, st ocm.Share_ShareType) (*ocm.Share, error) {
	type paramsObj struct {
		Md *provider.ResourceId `json:"md"`
		G  *ocm.ShareGrant      `json:"g"`
	}
	bodyObj := &paramsObj{
		Md: md,
		G:  g,
	}
	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}

	_, body, err := sm.do(ctx, Action{"Share", string(bodyStr)})

	if err != nil {
		return nil, err
	}

	altResult := &ShareAltMap{}
	err = json.Unmarshal(body, &altResult)
	if altResult == nil {
		return nil, err
	}
	return &ocm.Share{
		Id:          altResult.ID,
		ResourceId:  altResult.ResourceID,
		Permissions: altResult.Permissions,
		Grantee: &provider.Grantee{
			Id: altResult.Grantee.ID,
		},
		Owner:   altResult.Owner,
		Creator: altResult.Creator,
		Ctime:   altResult.Ctime,
		Mtime:   altResult.Mtime,
	}, err
}

// GetShare as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) GetShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.Share, error) {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}
	_, body, err := sm.do(ctx, Action{"GetShare", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	altResult := &ShareAltMap{}
	err = json.Unmarshal(body, &altResult)
	if altResult == nil {
		return nil, err
	}
	return &ocm.Share{
		Id:          altResult.ID,
		ResourceId:  altResult.ResourceID,
		Permissions: altResult.Permissions,
		Grantee: &provider.Grantee{
			Id: altResult.Grantee.ID,
		},
		Owner:   altResult.Owner,
		Creator: altResult.Creator,
		Ctime:   altResult.Ctime,
		Mtime:   altResult.Mtime,
	}, err
}

// Unshare as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) Unshare(ctx context.Context, ref *ocm.ShareReference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}

	_, _, err = sm.do(ctx, Action{"Unshare", string(bodyStr)})
	return err
}

// UpdateShare as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) UpdateShare(ctx context.Context, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error) {
	type paramsObj struct {
		Ref *ocm.ShareReference   `json:"ref"`
		P   *ocm.SharePermissions `json:"p"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
		P:   p,
	}
	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}

	_, body, err := sm.do(ctx, Action{"UpdateShare", string(bodyStr)})

	if err != nil {
		return nil, err
	}

	altResult := &ShareAltMap{}
	err = json.Unmarshal(body, &altResult)
	if altResult == nil {
		return nil, err
	}
	return &ocm.Share{
		Id:          altResult.ID,
		ResourceId:  altResult.ResourceID,
		Permissions: altResult.Permissions,
		Grantee: &provider.Grantee{
			Id: altResult.Grantee.ID,
		},
		Owner:   altResult.Owner,
		Creator: altResult.Creator,
		Ctime:   altResult.Ctime,
		Mtime:   altResult.Mtime,
	}, err
}

// ListShares as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) ListShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	bodyStr, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"ListShares", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	var respArr []ShareAltMap
	err = json.Unmarshal(respBody, &respArr)
	if err != nil {
		return nil, err
	}

	var pointers = make([]*ocm.Share, len(respArr))
	for i := 0; i < len(respArr); i++ {
		altResult := respArr[i]
		pointers[i] = &ocm.Share{
			Id:          altResult.ID,
			ResourceId:  altResult.ResourceID,
			Permissions: altResult.Permissions,
			Grantee: &provider.Grantee{
				Id: altResult.Grantee.ID,
			},
			Owner:   altResult.Owner,
			Creator: altResult.Creator,
			Ctime:   altResult.Ctime,
			Mtime:   altResult.Mtime,
		}
	}
	return pointers, err
}

// ListReceivedShares as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm *Manager) ListReceivedShares(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	_, respBody, err := sm.do(ctx, Action{"ListReceivedShares", string("")})
	if err != nil {
		return nil, err
	}

	var respArr []ReceivedShareAltMap
	err = json.Unmarshal(respBody, &respArr)
	if err != nil {
		return nil, err
	}
	var pointers = make([]*ocm.ReceivedShare, len(respArr))
	for i := 0; i < len(respArr); i++ {
		altResultShare := respArr[i].Share
		if altResultShare == nil {
			pointers[i] = &ocm.ReceivedShare{
				Share: nil,
				State: respArr[i].State,
			}
		} else {
			pointers[i] = &ocm.ReceivedShare{
				Share: &ocm.Share{
					Id:          altResultShare.ID,
					ResourceId:  altResultShare.ResourceID,
					Permissions: altResultShare.Permissions,
					Grantee: &provider.Grantee{
						Id: altResultShare.Grantee.ID,
					},
					Owner:   altResultShare.Owner,
					Creator: altResultShare.Creator,
					Ctime:   altResultShare.Ctime,
					Mtime:   altResultShare.Mtime,
				},
				State: respArr[i].State,
			}
		}
	}
	return pointers, err

}

// GetReceivedShare as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L29-L54
func (sm *Manager) GetReceivedShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"GetReceivedShare", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	var altResult ReceivedShareAltMap
	err = json.Unmarshal(respBody, &altResult)
	if err != nil {
		return nil, err
	}
	altResultShare := altResult.Share
	if altResultShare == nil {
		return &ocm.ReceivedShare{
			Share: nil,
			State: altResult.State,
		}, err
	}
	return &ocm.ReceivedShare{
		Share: &ocm.Share{
			Id:          altResultShare.ID,
			ResourceId:  altResultShare.ResourceID,
			Permissions: altResultShare.Permissions,
			Grantee: &provider.Grantee{
				Id: altResultShare.Grantee.ID,
			},
			Owner:   altResultShare.Owner,
			Creator: altResultShare.Creator,
			Ctime:   altResultShare.Ctime,
			Mtime:   altResultShare.Mtime,
		},
		State: altResult.State,
	}, err
}

// UpdateReceivedShare as defined in the ocm.share.Manager interface
// https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
func (sm Manager) UpdateReceivedShare(ctx context.Context, receivedShare *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	type paramsObj struct {
		ReceivedShare *ocm.ReceivedShare    `json:"received_share"`
		FieldMask     *field_mask.FieldMask `json:"field_mask"`
	}

	bodyObj := &paramsObj{
		ReceivedShare: receivedShare,
		FieldMask:     fieldMask,
	}
	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"UpdateReceivedShare", string(bodyStr)})
	if err != nil {
		return nil, err
	}

	var altResult ReceivedShareAltMap
	err = json.Unmarshal(respBody, &altResult)
	if err != nil {
		return nil, err
	}
	altResultShare := altResult.Share
	if altResultShare == nil {
		return &ocm.ReceivedShare{
			Share: nil,
			State: altResult.State,
		}, err
	}
	return &ocm.ReceivedShare{
		Share: &ocm.Share{
			Id:          altResultShare.ID,
			ResourceId:  altResultShare.ResourceID,
			Permissions: altResultShare.Permissions,
			Grantee: &provider.Grantee{
				Id: altResultShare.Grantee.ID,
			},
			Owner:   altResultShare.Owner,
			Creator: altResultShare.Creator,
			Ctime:   altResultShare.Ctime,
			Mtime:   altResultShare.Mtime,
		},
		State: altResult.State,
	}, err
}
