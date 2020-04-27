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

package filesystem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"sync"
	"time"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
	"github.com/golang/protobuf/jsonpb"
)

func init() {
	registry.Register("json", New)
}

// New returns a new filesystem public shares manager.
func New(c map[string]interface{}) (publicshare.Manager, error) {

	m := manager{
		mutex:       &sync.Mutex{},
		marshaler:   jsonpb.Marshaler{},
		unmarshaler: jsonpb.Unmarshaler{},
		file:        "/var/tmp/.publicshares", // TODO MUST be configurable.
	}

	fileContents, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}
	if len(fileContents) == 0 {
		err := ioutil.WriteFile(m.file, []byte("{}"), 0644)
		if err != nil {
			return nil, err
		}
	}

	return &m, nil
}

type manager struct {
	mutex *sync.Mutex
	file  string

	marshaler   jsonpb.Marshaler
	unmarshaler jsonpb.Unmarshaler
}

var (
	passwordProtected bool
)

// CreatePublicShare adds a new entry to manager.shares
func (m *manager) CreatePublicShare(ctx context.Context, u *user.User, rInfo *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {
	id := &link.PublicShareId{
		OpaqueId: randString(12),
	}

	tkn := randString(12)
	now := time.Now().UnixNano()

	displayName, ok := rInfo.ArbitraryMetadata.Metadata["name"]
	if !ok {
		displayName = tkn
	}

	if len(rInfo.ArbitraryMetadata.Metadata["password"]) > 0 {
		// TODO hash the password!
		g.Password, passwordProtected = rInfo.ArbitraryMetadata.Metadata["password"]
	}

	createdAt := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	modifiedAt := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	s := link.PublicShare{
		Id:                id,
		Owner:             rInfo.GetOwner(),
		Creator:           u.Id,
		ResourceId:        rInfo.Id,
		Token:             tkn,
		Permissions:       g.Permissions,
		Ctime:             createdAt,
		Mtime:             modifiedAt,
		PasswordProtected: passwordProtected,
		Expiration:        g.Expiration,
		DisplayName:       displayName,
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	buff := bytes.Buffer{}
	if err := m.marshaler.Marshal(&buff, &s); err != nil {
		return nil, err
	}

	db := map[string]interface{}{}
	fileContents, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(fileContents, &db); err != nil {
		return nil, err
	}

	if _, ok := db[s.Id.GetOpaqueId()]; !ok {
		db[s.Id.GetOpaqueId()] = buff.String()
	} else {
		return nil, errors.New("key already exists")
	}

	destJSON, err := json.Marshal(db)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(m.file, destJSON, 0644); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpdatePublicShare updates the public share
func (m *manager) UpdatePublicShare(ctx context.Context, u *user.User, req *link.UpdatePublicShareRequest, g *link.Grant) (*link.PublicShare, error) {
	log := appctx.GetLogger(ctx)
	share, err := m.GetPublicShare(ctx, u, req.Ref)
	if err != nil {
		return nil, errors.New("ref does not exist")
	}

	now := time.Now().UnixNano()

	switch req.GetUpdate().GetType() {
	case link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME:
		log.Debug().Str("memory", "update display name").Msgf("from: `%v` to `%v`", share.DisplayName, req.Update.GetDisplayName())
		share.DisplayName = req.Update.GetDisplayName()
	case link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS:
		old, _ := json.Marshal(share.Permissions)
		new, _ := json.Marshal(req.Update.GetGrant().Permissions)
		log.Debug().Str("memory", "update grants").Msgf("from: `%v`\nto\n`%v`", old, new)
		share.Permissions = req.Update.GetGrant().GetPermissions()
	case link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION:
		old, _ := json.Marshal(share.Expiration)
		new, _ := json.Marshal(req.Update.GetGrant().Expiration)
		log.Debug().Str("memory", "update expiration").Msgf("from: `%v`\nto\n`%v`", old, new)
		share.Expiration = req.Update.GetGrant().Expiration
	case link.UpdatePublicShareRequest_Update_TYPE_PASSWORD:
		fallthrough
	default:
		return nil, fmt.Errorf("invalid update type: %v", req.GetUpdate().GetType())
	}

	share.Mtime = &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db := map[string]interface{}{}
	fileBytes, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(fileBytes, &db); err != nil {
		return nil, err
	}

	buff := bytes.Buffer{}
	if err := m.marshaler.Marshal(&buff, share); err != nil {
		return nil, err
	}

	db[share.GetId().OpaqueId] = buff.String()

	dbAsJSON, err := json.Marshal(db)
	if err != nil {
		return nil, err
	}

	if err := ioutil.WriteFile(m.file, dbAsJSON, 0644); err != nil {
		return nil, err
	}

	return share, nil
}

// GetPublicShare gets a public share either by ID or Token.
func (m *manager) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) (share *link.PublicShare, err error) {
	if ref.GetToken() != "" {
		share, err = m.GetPublicShareByToken(ctx, ref.GetToken())
		if err != nil {
			return nil, errors.New("no shares found by token")
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db := map[string]interface{}{}
	fileBytes, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(fileBytes, &db); err != nil {
		return nil, err
	}

	if found, ok := db[ref.GetId().GetOpaqueId()]; ok {
		ps := link.PublicShare{}
		r := bytes.NewBuffer([]byte(found.(string)))
		if err := m.unmarshaler.Unmarshal(r, &ps); err != nil {
			return nil, err
		}

		return &ps, nil
	}

	return
}

// ListPublicShares retrieves all the shares on the manager that are valid.
func (m *manager) ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo) ([]*link.PublicShare, error) {
	shares := []*link.PublicShare{}
	now := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db := map[string]interface{}{}
	readBytes, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(readBytes, &db); err != nil {
		return nil, err
	}

	for _, v := range db {
		r := bytes.NewBuffer([]byte(v.(string)))
		local := &link.PublicShare{}
		if err := m.unmarshaler.Unmarshal(r, local); err != nil {
			return nil, err
		}

		if len(filters) == 0 {
			shares = append(shares, local)
		} else {
			for _, f := range filters {
				if f.Type == link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID {
					t := time.Unix(int64(local.Expiration.GetSeconds()), int64(local.Expiration.GetNanos()))
					if err != nil {
						return nil, err
					}
					if local.ResourceId.StorageId == f.GetResourceId().StorageId && local.ResourceId.OpaqueId == f.GetResourceId().OpaqueId {
						if (local.Expiration != nil && t.After(now)) || local.Expiration == nil {
							shares = append(shares, local)
						}
					}
				}
			}
		}
	}

	return shares, nil
}

// RevokePublicShare undocumented.
func (m *manager) RevokePublicShare(ctx context.Context, u *user.User, id string) error {
	return fmt.Errorf("RevokePublicShare method unimplemented")
}

// GetPublicShareByToken gets a public share by its opaque token.
func (m *manager) GetPublicShareByToken(ctx context.Context, token string) (*link.PublicShare, error) {
	db := map[string]interface{}{}
	readBytes, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(readBytes, &db); err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, v := range db {
		r := bytes.NewBuffer([]byte(v.(string)))
		local := &link.PublicShare{}
		if err := m.unmarshaler.Unmarshal(r, local); err != nil {
			return nil, err
		}

		if local.Token == token {
			return local, nil
		}
	}

	return nil, fmt.Errorf("share with token: `%v` not found", token)
}

// randString is a helper to create tokens. It could be a token manager instead.
func randString(n int) string {
	var l = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = l[rand.Intn(len(l))]
	}
	return string(b)
}
