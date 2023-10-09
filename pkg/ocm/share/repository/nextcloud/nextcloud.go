// Copyright 2018-2023 CERN
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
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/genproto/protobuf/field_mask"
)

func init() {
	registry.Register("nextcloud", New)
}

// Manager is the Nextcloud-based implementation of the share.Repository interface
// see https://github.com/cs3org/reva/blob/v1.13.0/pkg/ocm/share/share.go#L30-L57
type Manager struct {
	client       *http.Client
	sharedSecret string
	webDAVHost   string
	endPoint     string
	mountID      string
}

// ShareManagerConfig contains config for a Nextcloud-based ShareManager.
type ShareManagerConfig struct {
	EndPoint     string `docs:";The Nextcloud backend endpoint for user check"                                                                                         mapstructure:"endpoint"`
	SharedSecret string `mapstructure:"shared_secret"`
	WebDAVHost   string `mapstructure:"webdav_host"`
	MockHTTP     bool   `mapstructure:"mock_http"`
	MountID      string `docs:";The Reva mount id to identify the storage provider proxying the EFSS. Note that only one EFSS can be proxied by a given Reva process." mapstructure:"mount_id"`
}

// Action describes a REST request to forward to the Nextcloud backend.
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

// ShareAltMap is an alternative map to JSON-unmarshal a Share.
type ShareAltMap struct {
	ID         *ocm.ShareId `json:"id"`
	ResourceID struct {
		OpaqueID string `json:"opaque_id"`
	} `json:"resource_id"`
	RemoteShareID string `json:"remote_share_id"`
	Permissions   int    `json:"permissions"`
	Grantee       struct {
		ID *userpb.UserId `json:"id"`
	} `json:"grantee"`
	Owner   *userpb.User       `json:"owner"`
	Creator *userpb.User       `json:"creator"`
	Ctime   *typespb.Timestamp `json:"ctime"`
	Mtime   *typespb.Timestamp `json:"mtime"`
	Token   string             `json:"token"`
}

// ReceivedShareAltMap is an alternative map to JSON-unmarshal a ReceivedShare.
type ReceivedShareAltMap struct {
	Share *ShareAltMap   `json:"share"`
	State ocm.ShareState `json:"state"`
}

// New returns a share manager implementation that verifies against a Nextcloud backend.
func New(ctx context.Context, m map[string]interface{}) (share.Repository, error) {
	var c ShareManagerConfig
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	return NewShareManager(&c)
}

// NewShareManager returns a new Nextcloud-based ShareManager.
func NewShareManager(c *ShareManagerConfig) (*Manager, error) {
	var client *http.Client
	if c.MockHTTP {
		// called := make([]string, 0)
		// nextcloudServerMock := GetNextcloudServerMock(&called)
		// client, _ = TestingHTTPClient(nextcloudServerMock)

		// Wait for SetHTTPClient to be called later
		client = nil
	} else {
		if len(c.EndPoint) == 0 {
			return nil, errors.New("Please specify 'endpoint' in '[grpc.services.ocmshareprovider.drivers.nextcloud]' and  '[grpc.services.ocmcore.drivers.nextcloud]'")
		}
		client = &http.Client{}
	}

	return &Manager{
		endPoint:     c.EndPoint, // e.g. "http://nc/apps/sciencemesh/"
		sharedSecret: c.SharedSecret,
		client:       client,
		webDAVHost:   c.WebDAVHost,
		mountID:      c.MountID,
	}, nil
}

// SetHTTPClient sets the HTTP client.
func (sm *Manager) SetHTTPClient(c *http.Client) {
	sm.client = c
}

// StoreShare stores a share.
func (sm *Manager) StoreShare(ctx context.Context, share *ocm.Share) (*ocm.Share, error) {
	encShare, err := utils.MarshalProtoV1ToJSON(share)
	if err != nil {
		return nil, err
	}
	_, body, err := sm.do(ctx, Action{"addSentShare", string(encShare)}, getUsername(&userpb.User{Id: share.Creator}))
	if err != nil {
		return nil, err
	}
	share.Id = &ocm.ShareId{
		OpaqueId: string(body),
	}
	return share, nil
}

func (sm *Manager) efssShareToOcm(resp *ShareAltMap) *ocm.Share {
	// Parse the JSON struct returned by the PHP SM app into an OCM share object
	return &ocm.Share{
		Id: resp.ID,
		ResourceId: &provider.ResourceId{
			OpaqueId:  resp.ResourceID.OpaqueID,
			StorageId: sm.mountID,
		},
		Name:  "", // FIXME missing from SM app
		Token: resp.Token,
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			Id: &provider.Grantee_UserId{
				UserId: resp.Grantee.ID,
			},
		},
		Owner: &userpb.UserId{
			OpaqueId: resp.Owner.Id.OpaqueId,
			Idp:      resp.Owner.Id.Idp,
		},
		Creator: &userpb.UserId{
			OpaqueId: resp.Creator.Id.OpaqueId,
			Idp:      resp.Creator.Id.Idp,
		},
		Ctime:     resp.Ctime,
		Mtime:     resp.Mtime,
		ShareType: ocm.ShareType_SHARE_TYPE_USER,
		// FIXME the SM app does not provide methods and does not include permissions, see https://github.com/sciencemesh/nc-sciencemesh/issues/45
		// the correct logic here is to include those access methods that come in the payload
		AccessMethods: []*ocm.AccessMethod{
			// FIXME for webdav we should use conversions.RoleFromOCSPermissions(conversions.Permissions(resp.Permissions))).CS3ResourcePermissions()
			share.NewWebDavAccessMethod(conversions.NewEditorRole().CS3ResourcePermissions()),
			// FIXME add if apps are supported
			// share.NewWebappAccessMethod(appprovider.ViewMode_VIEW_MODE_READ_WRITE),
			// FIXME add if datatx are supported
			// share.NewTransferAccessMethod(),
		},
	}
}

// GetShare gets the information for a share by the given ref.
func (sm *Manager) GetShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.Share, error) {
	data, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}
	_, body, err := sm.do(ctx, Action{"GetSentShareByToken", string(data)}, getUsername(user))
	if err != nil {
		return nil, err
	}

	altResult := ShareAltMap{}
	if err := json.Unmarshal(body, &altResult); err != nil {
		return nil, err
	}
	return sm.efssShareToOcm(&altResult), nil
}

// DeleteShare deletes the share pointed by ref.
func (sm *Manager) DeleteShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) error {
	bodyStr, err := json.Marshal(ref)
	if err != nil {
		return err
	}

	_, _, err = sm.do(ctx, Action{"Unshare", string(bodyStr)}, getUsername(user))
	return err
}

// UpdateShare updates the mode of the given share.
func (sm *Manager) UpdateShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference, f ...*ocm.UpdateOCMShareRequest_UpdateField) (*ocm.Share, error) {
	type paramsObj struct {
		Ref *ocm.ShareReference   `json:"ref"`
		P   *ocm.SharePermissions `json:"p"`
	}
	bodyObj := &paramsObj{
		Ref: ref,
	}
	data, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}

	_, body, err := sm.do(ctx, Action{"UpdateShare", string(data)}, getUsername(user))
	if err != nil {
		return nil, err
	}

	altResult := ShareAltMap{}
	if err := json.Unmarshal(body, &altResult); err != nil {
		return nil, err
	}
	return sm.efssShareToOcm(&altResult), nil
}

// ListShares returns the shares created by the user. If md is provided is not nil,
// it returns only shares attached to the given resource.
func (sm *Manager) ListShares(ctx context.Context, user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	data, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"ListShares", string(data)}, getUsername(user))
	if err != nil {
		return nil, err
	}

	var respArr []ShareAltMap
	if err := json.Unmarshal(respBody, &respArr); err != nil {
		return nil, err
	}

	var lst = make([]*ocm.Share, 0, len(respArr))
	for _, altResult := range respArr {
		lst = append(lst, sm.efssShareToOcm(&altResult))
	}
	return lst, nil
}

// StoreReceivedShare stores a received share.
func (sm *Manager) StoreReceivedShare(ctx context.Context, share *ocm.ReceivedShare) (*ocm.ReceivedShare, error) {
	data, err := utils.MarshalProtoV1ToJSON(share)
	if err != nil {
		return nil, err
	}
	_, body, err := sm.do(ctx, Action{"addReceivedShare", string(data)}, getUsername(&userpb.User{Id: share.Grantee.GetUserId()}))
	if err != nil {
		return nil, err
	}
	share.Id = &ocm.ShareId{
		OpaqueId: string(body),
	}

	return share, nil
}

func efssReceivedShareToOcm(altResultShare *ReceivedShareAltMap) *ocm.ReceivedShare {
	// Parse the JSON struct returned by the PHP SM app into an OCM received share object
	return &ocm.ReceivedShare{
		Id:            altResultShare.Share.ID,
		Name:          "",                                 // FIXME missing on SM app
		RemoteShareId: altResultShare.Share.RemoteShareID, // sic, see https://github.com/cs3org/reva/pull/3852#discussion_r1189681465
		Grantee: &provider.Grantee{
			Id: &provider.Grantee_UserId{
				UserId: altResultShare.Share.Grantee.ID,
			},
		},
		Owner:     altResultShare.Share.Owner.Id,
		Creator:   altResultShare.Share.Creator.Id,
		Ctime:     altResultShare.Share.Ctime,
		Mtime:     altResultShare.Share.Mtime,
		ShareType: ocm.ShareType_SHARE_TYPE_USER,
		// ResourceType: provider.ResourceType_RESOURCE_TYPE_FILE or CONTAINER, missing info on SM app
		// Protocols: []*ocm.Protocol{}   FIXME SM app does not persist multi protocols yet
		State: altResultShare.State,
	}
}

// ListReceivedShares returns the list of shares the user has access.
func (sm *Manager) ListReceivedShares(ctx context.Context, user *userpb.User) ([]*ocm.ReceivedShare, error) {
	_, respBody, err := sm.do(ctx, Action{"ListReceivedShares", ""}, getUsername(user))
	if err != nil {
		return nil, err
	}

	var respArr []ReceivedShareAltMap
	if err := json.Unmarshal(respBody, &respArr); err != nil {
		return nil, err
	}

	res := make([]*ocm.ReceivedShare, 0, len(respArr))
	for _, share := range respArr {
		if share.Share != nil {
			res = append(res, efssReceivedShareToOcm(&share))
		}
	}
	return res, nil
}

// GetReceivedShare returns the information for a received share the user has access.
func (sm *Manager) GetReceivedShare(ctx context.Context, user *userpb.User, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	data, err := json.Marshal(ref)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"GetReceivedShare", string(data)}, getUsername(user))
	if err != nil {
		return nil, err
	}

	var altResult ReceivedShareAltMap
	if err := json.Unmarshal(respBody, &altResult); err != nil {
		return nil, err
	}
	altResultShare := altResult.Share
	if altResultShare == nil {
		return nil, errtypes.NotFound("Received share not found from EFSS API")
	}
	return efssReceivedShareToOcm(&altResult), nil
}

// UpdateReceivedShare updates the received share with share state.
func (sm *Manager) UpdateReceivedShare(ctx context.Context, user *userpb.User, share *ocm.ReceivedShare, fieldMask *field_mask.FieldMask) (*ocm.ReceivedShare, error) {
	type paramsObj struct {
		ReceivedShare *ocm.ReceivedShare    `json:"received_share"`
		FieldMask     *field_mask.FieldMask `json:"field_mask"`
	}

	bodyObj := &paramsObj{
		ReceivedShare: share,
		FieldMask:     fieldMask,
	}
	bodyStr, err := json.Marshal(bodyObj)
	if err != nil {
		return nil, err
	}

	_, respBody, err := sm.do(ctx, Action{"UpdateReceivedShare", string(bodyStr)}, getUsername(user))
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
		return nil, errtypes.NotFound("Received share not found from EFSS API")
	}
	return efssReceivedShareToOcm(&altResult), nil
}

func getUsername(user *userpb.User) string {
	if user != nil && len(user.Username) > 0 {
		return user.Username
	}
	if user != nil && len(user.Id.OpaqueId) > 0 {
		return user.Id.OpaqueId
	}

	return "nobody"
}

func (sm *Manager) do(ctx context.Context, a Action, username string) (int, []byte, error) {
	url := sm.endPoint + "~" + username + "/api/ocm/" + a.verb

	log := appctx.GetLogger(ctx)
	log.Info().Msgf("sm.do %s %s", url, a.argS)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(a.argS))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Reva-Secret", sm.sharedSecret)

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

	// curl -i -H 'application/json' -H 'X-Reva-Secret: shared-secret-1' -d '{"md":{"opaque_id":"fileid-/other/q/as"},"g":{"grantee":{"type":1,"Id":{"UserId":{"idp":"revanc2.docker","opaque_id":"marie"}}},"permissions":{"permissions":{"get_path":true,"initiate_file_download":true,"list_container":true,"list_file_versions":true,"stat":true}}},"provider_domain":"cern.ch","resource_type":"file","provider_id":2,"owner_opaque_id":"einstein","owner_display_name":"Albert Einstein","protocol":{"name":"webdav","options":{"sharedSecret":"secret","permissions":"webdav-property"}}}' https://nc1.docker/index.php/apps/sciencemesh/~/api/ocm/addSentShare

	log.Info().Int("status", resp.StatusCode).Msgf("sent request to EFSS API, response: %s", body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, nil, fmt.Errorf("Unexpected response from EFSS API: " + strconv.Itoa(resp.StatusCode))
	}
	return resp.StatusCode, body, nil
}
