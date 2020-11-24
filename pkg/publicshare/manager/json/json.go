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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
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
	"github.com/mitchellh/mapstructure"
)

func init() {
	registry.Register("json", New)
}

// New returns a new filesystem public shares manager.
func New(c map[string]interface{}) (publicshare.Manager, error) {
	conf := &config{}
	if err := mapstructure.Decode(c, conf); err != nil {
		return nil, err
	}

	conf.init()

	m := manager{
		mutex:       &sync.Mutex{},
		marshaler:   jsonpb.Marshaler{},
		unmarshaler: jsonpb.Unmarshaler{},
		file:        conf.File,
	}

	// attempt to create the db file
	var fi os.FileInfo
	var err error
	if fi, err = os.Stat(m.file); os.IsNotExist(err) {
		folder := filepath.Dir(m.file)
		if err := os.MkdirAll(folder, 0755); err != nil {
			return nil, err
		}
		if _, err := os.Create(m.file); err != nil {
			return nil, err
		}
	}

	if fi == nil || fi.Size() == 0 {
		err := ioutil.WriteFile(m.file, []byte("{}"), 0644)
		if err != nil {
			return nil, err
		}
	}

	return &m, nil
}

type config struct {
	File string `mapstructure:"file"`
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/publicshares"
	}
}

type manager struct {
	mutex *sync.Mutex
	file  string

	marshaler   jsonpb.Marshaler
	unmarshaler jsonpb.Unmarshaler
}

// CreatePublicShare adds a new entry to manager.shares
func (m *manager) CreatePublicShare(ctx context.Context, u *user.User, rInfo *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {
	id := &link.PublicShareId{
		OpaqueId: randString(15),
	}

	tkn := randString(15)
	now := time.Now().UnixNano()

	displayName, ok := rInfo.ArbitraryMetadata.Metadata["name"]
	if !ok {
		displayName = tkn
	}

	var passwordProtected bool
	password := g.Password
	if len(password) > 0 {
		password = base64.StdEncoding.EncodeToString([]byte(password))
		passwordProtected = true
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

	ps := &publicShare{
		PublicShare: s,
		Password:    password,
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	encShare := bytes.Buffer{}
	if err := m.marshaler.Marshal(&encShare, &ps.PublicShare); err != nil {
		return nil, err
	}

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	if _, ok := db[s.Id.GetOpaqueId()]; !ok {
		db[s.Id.GetOpaqueId()] = map[string]interface{}{
			"share":    encShare.String(),
			"password": ps.Password,
		}
	} else {
		return nil, errors.New("key already exists")
	}

	err = m.writeDb(db)
	if err != nil {
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
	var newPasswordEncoded string
	passwordChanged := false

	switch req.GetUpdate().GetType() {
	case link.UpdatePublicShareRequest_Update_TYPE_DISPLAYNAME:
		log.Debug().Str("json", "update display name").Msgf("from: `%v` to `%v`", share.DisplayName, req.Update.GetDisplayName())
		share.DisplayName = req.Update.GetDisplayName()
	case link.UpdatePublicShareRequest_Update_TYPE_PERMISSIONS:
		old, _ := json.Marshal(share.Permissions)
		new, _ := json.Marshal(req.Update.GetGrant().Permissions)
		log.Debug().Str("json", "update grants").Msgf("from: `%v`\nto\n`%v`", old, new)
		share.Permissions = req.Update.GetGrant().GetPermissions()
	case link.UpdatePublicShareRequest_Update_TYPE_EXPIRATION:
		old, _ := json.Marshal(share.Expiration)
		new, _ := json.Marshal(req.Update.GetGrant().Expiration)
		log.Debug().Str("json", "update expiration").Msgf("from: `%v`\nto\n`%v`", old, new)
		share.Expiration = req.Update.GetGrant().Expiration
	case link.UpdatePublicShareRequest_Update_TYPE_PASSWORD:
		passwordChanged = true
		if req.Update.GetGrant().Password == "" {
			share.PasswordProtected = false
			newPasswordEncoded = ""
		} else {
			newPasswordEncoded = base64.StdEncoding.EncodeToString([]byte(req.Update.GetGrant().Password))
			share.PasswordProtected = true
		}
	default:
		return nil, fmt.Errorf("invalid update type: %v", req.GetUpdate().GetType())
	}

	share.Mtime = &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	buff := bytes.Buffer{}
	if err := m.marshaler.Marshal(&buff, share); err != nil {
		return nil, err
	}

	data, ok := db[share.Id.OpaqueId].(map[string]interface{})
	if !ok {
		data = map[string]interface{}{}
	}

	if ok && passwordChanged {
		data["password"] = newPasswordEncoded
	}
	data["share"] = buff.String()

	db[share.Id.OpaqueId] = data

	err = m.writeDb(db)
	if err != nil {
		return nil, err
	}

	return share, nil
}

// GetPublicShare gets a public share either by ID or Token.
func (m *manager) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) (*link.PublicShare, error) {
	if ref.GetToken() != "" {
		ps, err := m.getByToken(ctx, ref.GetToken())
		if err != nil {
			return nil, errors.New("no shares found by token")
		}
		return ps, nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	// compare ref opaque id with share opaqueid
	for _, v := range db {
		// db[ref.GetId().GetOpaqueId()].(map[string]interface{})["share"]
		// fmt.Printf("\nHERE\n%v\n\n", v.(map[string]interface{})["share"])
		d := v.(map[string]interface{})["share"]

		ps := &link.PublicShare{}
		r := bytes.NewBuffer([]byte(d.(string)))
		if err := m.unmarshaler.Unmarshal(r, ps); err != nil {
			return nil, err
		}

		if ref.GetId().GetOpaqueId() == ps.Id.OpaqueId {
			return ps, nil
		}

	}
	return nil, errors.New("no shares found by id:" + ref.GetId().String())

	// found, ok := db[ref.GetId().GetOpaqueId()].(map[string]interface{})["share"]
	// if !ok {
	// 	return nil, errors.New("no shares found by id:" + ref.GetId().String())
	// }

	// ps := publicShare{}
	// r := bytes.NewBuffer([]byte(found.(string)))
	// if err := m.unmarshaler.Unmarshal(r, &ps); err != nil {
	// 	return nil, err
	// }

	// return &ps.PublicShare, nil

}

// ListPublicShares retrieves all the shares on the manager that are valid.
func (m *manager) ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo) ([]*link.PublicShare, error) {
	shares := []*link.PublicShare{}
	now := time.Now()

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	for _, v := range db {
		r := bytes.NewBuffer([]byte(v.(map[string]interface{})["share"].(string)))
		local := &publicShare{}
		if err := m.unmarshaler.Unmarshal(r, &local.PublicShare); err != nil {
			return nil, err
		}

		// Skip if the share isn't created by the current user
		if local.Creator.GetOpaqueId() != u.Id.OpaqueId || (local.Creator.GetIdp() != "" && u.Id.Idp != local.Creator.GetIdp()) {
			continue
		}

		if len(filters) == 0 {
			shares = append(shares, &local.PublicShare)
		} else {
			for _, f := range filters {
				if f.Type == link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID {
					t := time.Unix(int64(local.Expiration.GetSeconds()), int64(local.Expiration.GetNanos()))
					if err != nil {
						return nil, err
					}
					if local.ResourceId.StorageId == f.GetResourceId().StorageId && local.ResourceId.OpaqueId == f.GetResourceId().OpaqueId {
						if (local.Expiration != nil && t.After(now)) || local.Expiration == nil {
							shares = append(shares, &local.PublicShare)
						}
					}
				}
			}
		}
	}

	return shares, nil
}

// RevokePublicShare undocumented.
func (m *manager) RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return err
	}

	switch {
	case ref.GetId().OpaqueId != "":
		delete(db, ref.GetId().OpaqueId)
	case ref.GetToken() != "":
		share, err := m.getByToken(ctx, ref.GetToken())
		if err != nil {
			return err
		}
		delete(db, share.Id.OpaqueId)
	default:
		return errors.New("reference does not exist")
	}

	return m.writeDb(db)
}

func (m *manager) getByToken(ctx context.Context, token string) (*link.PublicShare, error) {
	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, v := range db {
		r := bytes.NewBuffer([]byte(v.(map[string]interface{})["share"].(string)))
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

// GetPublicShareByToken gets a public share by its opaque token.
func (m *manager) GetPublicShareByToken(ctx context.Context, token, password string) (*link.PublicShare, error) {
	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, v := range db {
		r := bytes.NewBuffer([]byte(v.(map[string]interface{})["share"].(string)))
		passDB := v.(map[string]interface{})["password"].(string)
		local := &link.PublicShare{}
		if err := m.unmarshaler.Unmarshal(r, local); err != nil {
			return nil, err
		}

		if local.Token == token {
			// validate if it is password protected
			if local.PasswordProtected {
				password = base64.StdEncoding.EncodeToString([]byte(password))
				// check sent password matches stored one
				if passDB == password {
					return local, nil
				}
				// TODO(refs): custom permission denied error to catch up
				// in upper layers
				return nil, errors.New("json: invalid password")
			}
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

func (m *manager) readDb() (map[string]interface{}, error) {
	db := map[string]interface{}{}
	readBytes, err := ioutil.ReadFile(m.file)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(readBytes, &db); err != nil {
		return nil, err
	}
	return db, nil
}

func (m *manager) writeDb(db map[string]interface{}) error {
	dbAsJSON, err := json.Marshal(db)
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(m.file, dbAsJSON, 0644); err != nil {
		return err
	}

	return nil
}

type publicShare struct {
	link.PublicShare
	Password string `json:"password"`
}
