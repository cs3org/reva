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

package json

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
	"github.com/cs3org/reva/pkg/rhttp"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const createOCMCoreShareEndpoint = "shares"

func init() {
	registry.Register("json", New)
}

// New returns a new authorizer object.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}
	c.init()

	// load or create file
	model, err := loadOrCreate(c.File)
	if err != nil {
		err = errors.Wrap(err, "error loading the file containing the shares")
		return nil, err
	}

	mgr := &mgr{
		c:     c,
		model: model,
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(5 * time.Second),
		),
	}

	return mgr, nil
}

func loadOrCreate(file string) (*shareModel, error) {
	_, err := os.Stat(file)
	if os.IsNotExist(err) {
		if err := ioutil.WriteFile(file, []byte("{}"), 0700); err != nil {
			err = errors.Wrap(err, "error creating the file: "+file)
			return nil, err
		}
	}

	fd, err := os.OpenFile(file, os.O_CREATE, 0644)
	if err != nil {
		err = errors.Wrap(err, "error opening the file: "+file)
		return nil, err
	}
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return nil, err
	}

	m := &shareModel{}
	if err := json.Unmarshal(data, m); err != nil {
		err = errors.Wrap(err, "error decoding data to json")
		return nil, err
	}

	if m.State == nil {
		m.State = map[string]map[string]ocm.ShareState{}
	}
	m.file = file

	return m, nil
}

type shareModel struct {
	file           string
	State          map[string]map[string]ocm.ShareState `json:"state"` // map[username]map[share_id]boolean
	Shares         []*ocm.Share                         `json:"shares"`
	ReceivedShares []*ocm.Share                         `json:"received_shares"`
}

type config struct {
	File                string `mapstructure:"file"`
	InsecureConnections bool   `mapstructure:"insecure_connections"`
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/ocm-shares.json"
	}
}

type mgr struct {
	c          *config
	sync.Mutex // concurrent access to the file
	model      *shareModel
	client     *http.Client
}

func (m *shareModel) Save() error {
	data, err := json.Marshal(m)
	if err != nil {
		err = errors.Wrap(err, "error encoding to json")
		return err
	}

	if err := ioutil.WriteFile(m.file, data, 0644); err != nil {
		err = errors.Wrap(err, "error writing to file: "+m.file)
		return err
	}

	return nil
}

func (m *shareModel) ReadFile() error {
	data, err := ioutil.ReadFile(m.file)
	if err != nil {
		err = errors.Wrap(err, "error reading the data")
		return err
	}

	if err := json.Unmarshal(data, m); err != nil {
		err = errors.Wrap(err, "error decoding data to json")
		return err
	}

	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func genID() string {
	return uuid.New().String()
}

func getOCMEndpoint(originProvider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range originProvider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Endpoint.Path, nil
		}
	}
	return "", errors.New("json: ocm endpoint not specified for mesh provider")
}

func (m *mgr) Share(ctx context.Context, md *provider.ResourceId, g *ocm.ShareGrant, name string,
	pi *ocmprovider.ProviderInfo, pm string, owner *userpb.UserId, token string) (*ocm.Share, error) {

	id := genID()
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	// Since both OCMCore and OCMShareProvider use the same package, we distinguish
	// between calls received from them on the basis of whether they provide info
	// about the remote provider on which the share is to be created.
	// If this info is provided, this call is on the owner's mesh provider and so
	// we call the CreateOCMCoreShare method on the remote provider as well,
	// else this is received from another provider and we only create a local share.
	var isOwnersMeshProvider bool
	if pi != nil {
		isOwnersMeshProvider = true
	}

	var userID *userpb.UserId
	if !isOwnersMeshProvider {
		// Since this call is on the remote provider, the owner of the resource is expected to be specified.
		if owner == nil {
			return nil, errors.New("json: owner of resource not provided")
		}
		userID = owner
		g.Grantee.Opaque = &typespb.Opaque{
			Map: map[string]*typespb.OpaqueEntry{
				"token": &typespb.OpaqueEntry{
					Decoder: "plain",
					Value:   []byte(token),
				},
			},
		}
	} else {
		userID = user.ContextMustGetUser(ctx).GetId()
	}

	// do not allow share to myself if share is for a user
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(g.Grantee.GetUserId(), userID) {
		return nil, errors.New("json: user and grantee are the same")
	}

	// check if share already exists.
	key := &ocm.ShareKey{
		Owner:      userID,
		ResourceId: md,
		Grantee:    g.Grantee,
	}
	_, err := m.getByKey(ctx, key)

	// share already exists
	if isOwnersMeshProvider && err == nil {
		return nil, errtypes.AlreadyExists(key.String())
	}

	s := &ocm.Share{
		Id: &ocm.ShareId{
			OpaqueId: id,
		},
		Name:        name,
		ResourceId:  md,
		Permissions: g.Permissions,
		Grantee:     g.Grantee,
		Owner:       userID,
		Creator:     userID,
		Ctime:       ts,
		Mtime:       ts,
	}

	if isOwnersMeshProvider {

		// Call the remote provider's CreateOCMCoreShare method
		protocol, err := json.Marshal(
			map[string]interface{}{
				"name": "webdav",
				"options": map[string]string{
					"permissions": pm,
					"token":       tokenpkg.ContextMustGetToken(ctx),
				},
			},
		)
		if err != nil {
			err = errors.Wrap(err, "error marshalling protocol data")
			return nil, err
		}

		requestBody := url.Values{
			"shareWith":    {g.Grantee.GetUserId().OpaqueId},
			"name":         {name},
			"providerId":   {fmt.Sprintf("%s:%s", md.StorageId, md.OpaqueId)},
			"owner":        {userID.OpaqueId},
			"protocol":     {string(protocol)},
			"meshProvider": {userID.Idp},
		}

		ocmEndpoint, err := getOCMEndpoint(pi)
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(ocmEndpoint)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, createOCMCoreShareEndpoint)
		recipientURL := u.String()

		req, err := http.NewRequest("POST", recipientURL, strings.NewReader(requestBody.Encode()))
		if err != nil {
			return nil, errors.Wrap(err, "json: error framing post request")
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; param=value")

		resp, err := m.client.Do(req)
		if err != nil {
			err = errors.Wrap(err, "json: error sending post request")
			return nil, err
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			respBody, e := ioutil.ReadAll(resp.Body)
			if e != nil {
				e = errors.Wrap(e, "json: error reading request body")
				return nil, e
			}
			err = errors.Wrap(errors.New(fmt.Sprintf("%s: %s", resp.Status, string(respBody))), "json: error sending create ocm core share post request")
			return nil, err
		}
	}

	m.Lock()
	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}
	if isOwnersMeshProvider {
		m.model.Shares = append(m.model.Shares, s)
	} else {
		m.model.ReceivedShares = append(m.model.ReceivedShares, s)
	}

	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}
	m.Unlock()

	return s, nil
}

func (m *mgr) getByID(ctx context.Context, id *ocm.ShareId) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	for _, s := range m.model.Shares {
		if s.GetId().OpaqueId == id.OpaqueId {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *mgr) getByKey(ctx context.Context, key *ocm.ShareKey) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	for _, s := range m.model.Shares {
		if (utils.UserEqual(key.Owner, s.Owner) || utils.UserEqual(key.Owner, s.Creator)) &&
			utils.ResourceEqual(key.ResourceId, s.ResourceId) && utils.GranteeEqual(key.Grantee, s.Grantee) {
			return s, nil
		}
	}
	return nil, errtypes.NotFound(key.String())
}

func (m *mgr) get(ctx context.Context, ref *ocm.ShareReference) (s *ocm.Share, err error) {
	switch {
	case ref.GetId() != nil:
		s, err = m.getByID(ctx, ref.GetId())
	case ref.GetKey() != nil:
		s, err = m.getByKey(ctx, ref.GetKey())
	default:
		err = errtypes.NotFound(ref.String())
	}

	if err != nil {
		return nil, err
	}

	// check if we are the owner
	user := user.ContextMustGetUser(ctx)
	if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
		return s, nil
	}

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) GetShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.Share, error) {
	share, err := m.get(ctx, ref)
	if err != nil {
		return nil, err
	}

	return share, nil
}

func (m *mgr) Unshare(ctx context.Context, ref *ocm.ShareReference) error {
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return err
	}

	user := user.ContextMustGetUser(ctx)
	for i, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				m.model.Shares[len(m.model.Shares)-1], m.model.Shares[i] = m.model.Shares[i], m.model.Shares[len(m.model.Shares)-1]
				m.model.Shares = m.model.Shares[:len(m.model.Shares)-1]
				if err := m.model.Save(); err != nil {
					err = errors.Wrap(err, "error saving model")
					return err
				}
				return nil
			}
		}
	}
	return errtypes.NotFound(ref.String())
}

func sharesEqual(ref *ocm.ShareReference, s *ocm.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if (utils.UserEqual(ref.GetKey().Owner, s.Owner) || utils.UserEqual(ref.GetKey().Owner, s.Creator)) &&
			utils.ResourceEqual(ref.GetKey().ResourceId, s.ResourceId) && utils.GranteeEqual(ref.GetKey().Grantee, s.Grantee) {
			return true
		}
	}
	return false
}

func (m *mgr) UpdateShare(ctx context.Context, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	for i, s := range m.model.Shares {
		if sharesEqual(ref, s) {
			if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
				now := time.Now().UnixNano()
				m.model.Shares[i].Permissions = p
				m.model.Shares[i].Mtime = &typespb.Timestamp{
					Seconds: uint64(now / 1000000000),
					Nanos:   uint32(now % 1000000000),
				}
				if err := m.model.Save(); err != nil {
					err = errors.Wrap(err, "error saving model")
					return nil, err
				}
				return m.model.Shares[i], nil
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) ListShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	var ss []*ocm.Share
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.Shares {
		if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
			// no filter we return earlier
			if len(filters) == 0 {
				ss = append(ss, s)
			} else {
				// check filters
				// TODO(labkode): add the rest of filters.
				for _, f := range filters {
					if f.Type == ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID {
						if s.ResourceId.StorageId == f.GetResourceId().StorageId && s.ResourceId.OpaqueId == f.GetResourceId().OpaqueId {
							ss = append(ss, s)
						}
					}
				}
			}
		}
	}
	return ss, nil
}

func (m *mgr) ListReceivedShares(ctx context.Context) ([]*ocm.ReceivedShare, error) {
	var rss []*ocm.ReceivedShare
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.ReceivedShares {
		if utils.UserEqual(user.Id, s.Owner) || utils.UserEqual(user.Id, s.Creator) {
			// omit shares created by me
			continue
		}
		if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
			rs := m.convert(ctx, s)
			rss = append(rss, rs)
		} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			// check if all user groups match this share; TODO(labkode): filter shares created by us.
			for _, g := range user.Groups {
				if g == s.Grantee.GetGroupId().OpaqueId {
					rs := m.convert(ctx, s)
					rss = append(rss, rs)
					break
				}
			}
		}
	}
	return rss, nil
}

// convert must be called in a lock-controlled block.
func (m *mgr) convert(ctx context.Context, s *ocm.Share) *ocm.ReceivedShare {
	rs := &ocm.ReceivedShare{
		Share: s,
		State: ocm.ShareState_SHARE_STATE_PENDING,
	}
	user := user.ContextMustGetUser(ctx)
	if v, ok := m.model.State[user.Id.String()]; ok {
		if state, ok := v[s.Id.String()]; ok {
			rs.State = state
		}
	}
	return rs
}

func (m *mgr) GetReceivedShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	return m.getReceived(ctx, ref)
}

func (m *mgr) getReceived(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	for _, s := range m.model.ReceivedShares {
		if sharesEqual(ref, s) {
			if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER && utils.UserEqual(user.Id, s.Grantee.GetUserId()) {
				rs := m.convert(ctx, s)
				return rs, nil
			} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
				for _, g := range user.Groups {
					if s.Grantee.GetGroupId().OpaqueId == g {
						rs := m.convert(ctx, s)
						return rs, nil
					}
				}
			}
		}
	}
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, ref *ocm.ShareReference, f *ocm.UpdateReceivedOCMShareRequest_UpdateField) (*ocm.ReceivedShare, error) {
	rs, err := m.getReceived(ctx, ref)
	if err != nil {
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)
	m.Lock()
	defer m.Unlock()

	if err := m.model.ReadFile(); err != nil {
		err = errors.Wrap(err, "error reading model")
		return nil, err
	}

	if v, ok := m.model.State[user.Id.String()]; ok {
		v[rs.Share.Id.String()] = f.GetState()
		m.model.State[user.Id.String()] = v
	} else {
		a := map[string]ocm.ShareState{
			rs.Share.Id.String(): f.GetState(),
		}
		m.model.State[user.Id.String()] = a
	}

	if err := m.model.Save(); err != nil {
		err = errors.Wrap(err, "error saving model")
		return nil, err
	}

	return rs, nil
}
