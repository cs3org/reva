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

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"reflect"
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
	"github.com/cs3org/reva/pkg/rhttp"
	tokenpkg "github.com/cs3org/reva/pkg/token"
	"github.com/cs3org/reva/pkg/user"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

const createOCMCoreShareEndpoint = "shares"

func init() {
	// Don't use memory driver as we can't retrieve received shares.
	// registry.Register("memory", New)
}

// New returns a new memory manager.
func New(m map[string]interface{}) (share.Manager, error) {
	c, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "error creating a new manager")
		return nil, err
	}

	state := make(map[string]map[string]ocm.ShareState)
	return &mgr{
		c:      c,
		shares: sync.Map{},
		state:  state,
		client: rhttp.GetHTTPClient(
			rhttp.Timeout(5 * time.Second),
		),
	}, nil
}

type mgr struct {
	c      *config
	shares sync.Map
	state  map[string]map[string]ocm.ShareState
	client *http.Client
}

type config struct {
	InsecureConnections bool `mapstructure:"insecure_connections"`
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
	return "", errors.New("memory: ocm endpoint not specified for mesh provider")
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
	if g.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
		g.Grantee.Id.Idp == userID.Idp && g.Grantee.Id.OpaqueId == userID.OpaqueId {
		return nil, errors.New("json: user and grantee are the same")
	}

	// check if share already exists.
	key := &ocm.ShareKey{
		Owner:      userID,
		ResourceId: md,
		Grantee:    g.Grantee,
	}

	// share already exists
	_, ok := m.shares.Load(key)
	if isOwnersMeshProvider && ok {
		return nil, errtypes.AlreadyExists(key.String())
	}

	// Store share
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

	m.shares.Store(key, s)

	if isOwnersMeshProvider {

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
			"shareWith":    {g.Grantee.Id.OpaqueId},
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
			err = errors.Wrap(err, "memory: error sending post request")
			return nil, err
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			err = errors.Wrap(errors.New(resp.Status), "memory: error sending create ocm core share post request")
			return nil, err
		}
	}

	return s, nil
}

func (m *mgr) GetShare(ctx context.Context, ref *ocm.ShareReference) (s *ocm.Share, err error) {

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
	if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
		return s, nil
	}

	// we return not found to not disclose information
	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) getByID(ctx context.Context, id *ocm.ShareId) (*ocm.Share, error) {

	// iterate over existing shares and return the first one matching the id
	var found *ocm.Share
	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if s.GetId().OpaqueId == id.OpaqueId {
			found = v.(*ocm.Share)
			return true
		}

		return false
	})

	if found != nil {
		return found, nil
	}
	return nil, errtypes.NotFound(id.String())
}

func (m *mgr) getByKey(ctx context.Context, key *ocm.ShareKey) (*ocm.Share, error) {

	// iterate over existing shares and return the first one matching the key
	var found *ocm.Share
	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if key.Owner.Idp == s.Owner.Idp && key.Owner.OpaqueId == s.Owner.OpaqueId &&
			key.ResourceId.StorageId == s.ResourceId.StorageId && key.ResourceId.OpaqueId == s.ResourceId.OpaqueId &&
			key.Grantee.Type == s.Grantee.Type && key.Grantee.Id.Idp == s.Grantee.Id.Idp && key.Grantee.Id.OpaqueId == s.Grantee.Id.OpaqueId {

			found = v.(*ocm.Share)
			return true
		}

		return false
	})

	if found != nil {
		return found, nil
	}

	return nil, errtypes.NotFound(key.String())
}

func (m *mgr) Unshare(ctx context.Context, ref *ocm.ShareReference) error {

	var ctxUser = user.ContextMustGetUser(ctx)
	var key *ocm.ShareKey

	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if equal(ref, s) {
			if ctxUser.Id.Idp == s.Owner.Idp && ctxUser.Id.OpaqueId == s.Owner.OpaqueId {
				key = &ocm.ShareKey{
					Owner:      ctxUser.Id,
					ResourceId: s.ResourceId,
					Grantee:    s.Grantee,
				}
				return true
			}
		}
		return false
	})

	if key != nil {
		m.shares.Delete(key)
		return nil
	}

	return errtypes.NotFound(ref.String())
}

func equal(ref *ocm.ShareReference, s *ocm.Share) bool {
	if ref.GetId() != nil && s.Id != nil {
		if ref.GetId().OpaqueId == s.Id.OpaqueId {
			return true
		}
	} else if ref.GetKey() != nil {
		if reflect.DeepEqual(*ref.GetKey().Owner, *s.Owner) && reflect.DeepEqual(*ref.GetKey().ResourceId, *s.ResourceId) && reflect.DeepEqual(*ref.GetKey().Grantee, *s.Grantee) {
			return true
		}
	}
	return false
}

func (m *mgr) UpdateShare(ctx context.Context, ref *ocm.ShareReference, p *ocm.SharePermissions) (*ocm.Share, error) {

	var user = user.ContextMustGetUser(ctx)
	var key *ocm.ShareKey

	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if equal(ref, s) {
			if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
				key = &ocm.ShareKey{
					Owner:      user.Id,
					ResourceId: s.ResourceId,
					Grantee:    s.Grantee,
				}
				return true
			}
		}
		return false
	})

	if key != nil {

		s, ok := m.shares.Load(key)
		if ok {

			now := time.Now().UnixNano()
			share := s.(*ocm.Share)

			share.Permissions = p
			share.Mtime = &typespb.Timestamp{
				Seconds: uint64(now / 1000000000),
				Nanos:   uint32(now % 1000000000),
			}

			m.shares.Delete(key)
			m.shares.Store(key, share)
			return share, nil
		}
	}

	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) ListShares(ctx context.Context, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {

	user := user.ContextMustGetUser(ctx)
	shares, err := m.listShares(user, filters)
	return shares, err
}

func (m *mgr) listShares(user *userpb.User, filters []*ocm.ListOCMSharesRequest_Filter) ([]*ocm.Share, error) {
	var shares []*ocm.Share
	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// no filter we return earlier
			if len(filters) == 0 {
				shares = append(shares, s)
			} else {

				// check filters
				for _, f := range filters {
					if f.Type == ocm.ListOCMSharesRequest_Filter_TYPE_RESOURCE_ID {
						if s.ResourceId.StorageId == f.GetResourceId().StorageId && s.ResourceId.OpaqueId == f.GetResourceId().OpaqueId {
							shares = append(shares, s)
						}
					}
				}
			}
		}

		return true
	})

	return shares, nil
}

func (m *mgr) ListReceivedShares(ctx context.Context) ([]*ocm.ReceivedShare, error) {

	var receivedShares []*ocm.ReceivedShare
	user := user.ContextMustGetUser(ctx)

	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if user.Id.Idp == s.Owner.Idp && user.Id.OpaqueId == s.Owner.OpaqueId {
			// omit shares created by me
			// TODO(labkode): apply check for s.Creator also.
			return true
		}
		if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER {
			if user.Id.Idp == s.Grantee.Id.Idp && user.Id.OpaqueId == s.Grantee.Id.OpaqueId {
				rs := m.convert(ctx, s)
				receivedShares = append(receivedShares, rs)
			}
		} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
			// check if all user groups match this share;
			// TODO(labkode): filter shares created by us.
			for _, g := range user.Groups {
				if g == s.Grantee.Id.OpaqueId {
					rs := m.convert(ctx, s)
					receivedShares = append(receivedShares, rs)
				}
			}
		}

		return true
	})

	return receivedShares, nil
}

// convert must be called in a lock-controlled block.
func (m *mgr) convert(ctx context.Context, s *ocm.Share) *ocm.ReceivedShare {
	rs := &ocm.ReceivedShare{
		Share: s,
		State: ocm.ShareState_SHARE_STATE_PENDING,
	}
	user := user.ContextMustGetUser(ctx)
	if v, ok := m.state[user.Id.String()]; ok {
		if state, ok := v[s.Id.String()]; ok {
			rs.State = state
		}
	}
	return rs
}

func (m *mgr) GetReceivedShare(ctx context.Context, ref *ocm.ShareReference) (*ocm.ReceivedShare, error) {

	user := user.ContextMustGetUser(ctx)

	var found *ocm.ReceivedShare
	m.shares.Range(func(k, v interface{}) bool {

		s := v.(*ocm.Share)

		if equal(ref, s) {
			if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_USER &&
				s.Grantee.Id.Idp == user.Id.Idp && s.Grantee.Id.OpaqueId == user.Id.OpaqueId {
				found = m.convert(ctx, s)
				return true
			} else if s.Grantee.Type == provider.GranteeType_GRANTEE_TYPE_GROUP {
				for _, g := range user.Groups {
					if s.Grantee.Id.OpaqueId == g {
						found = m.convert(ctx, s)
						return true
					}
				}
			}
		}

		return false
	})

	if found != nil {
		return found, nil
	}

	return nil, errtypes.NotFound(ref.String())
}

func (m *mgr) UpdateReceivedShare(ctx context.Context, ref *ocm.ShareReference, f *ocm.UpdateReceivedOCMShareRequest_UpdateField) (*ocm.ReceivedShare, error) {

	rs, err := m.GetReceivedShare(ctx, ref)
	if err != nil {
		return nil, err
	}

	user := user.ContextMustGetUser(ctx)

	if v, ok := m.state[user.Id.String()]; ok {
		v[rs.Share.Id.String()] = f.GetState()
		m.state[user.Id.String()] = v
	} else {
		a := map[string]ocm.ShareState{
			rs.Share.Id.String(): f.GetState(),
		}
		m.state[user.Id.String()] = a
	}

	return rs, nil
}
