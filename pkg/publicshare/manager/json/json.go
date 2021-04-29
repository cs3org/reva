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
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/publicshare"
	"github.com/cs3org/reva/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
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
		mutex:                      &sync.Mutex{},
		file:                       conf.File,
		passwordHashCost:           conf.SharePasswordHashCost,
		janitorRunInterval:         conf.JanitorRunInterval,
		enableExpiredSharesCleanup: conf.EnableExpiredSharesCleanup,
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

	go m.startJanitorRun()

	return &m, nil
}

type config struct {
	File                       string `mapstructure:"file"`
	SharePasswordHashCost      int    `mapstructure:"password_hash_cost"`
	JanitorRunInterval         int    `mapstructure:"janitor_run_interval"`
	EnableExpiredSharesCleanup bool   `mapstructure:"enable_expired_shares_cleanup"`
}

func (c *config) init() {
	if c.File == "" {
		c.File = "/var/tmp/reva/publicshares"
	}
	if c.SharePasswordHashCost == 0 {
		c.SharePasswordHashCost = 11
	}
	if c.JanitorRunInterval == 0 {
		c.JanitorRunInterval = 60
	}
}

type manager struct {
	mutex *sync.Mutex
	file  string

	passwordHashCost           int
	janitorRunInterval         int
	enableExpiredSharesCleanup bool
}

func (m *manager) startJanitorRun() {
	if !m.enableExpiredSharesCleanup {
		return
	}

	ticker := time.NewTicker(time.Duration(m.janitorRunInterval) * time.Second)
	work := make(chan os.Signal, 1)
	signal.Notify(work, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)

	for {
		select {
		case <-work:
			return
		case <-ticker.C:
			m.cleanupExpiredShares()
		}
	}
}

// CreatePublicShare adds a new entry to manager.shares
func (m *manager) CreatePublicShare(ctx context.Context, u *user.User, rInfo *provider.ResourceInfo, g *link.Grant) (*link.PublicShare, error) {
	id := &link.PublicShareId{
		OpaqueId: utils.RandString(15),
	}

	tkn := utils.RandString(15)
	now := time.Now().UnixNano()

	displayName, ok := rInfo.ArbitraryMetadata.Metadata["name"]
	if !ok {
		displayName = tkn
	}

	var passwordProtected bool
	password := g.Password
	if len(password) > 0 {
		h, err := bcrypt.GenerateFromPassword([]byte(password), m.passwordHashCost)
		if err != nil {
			return nil, errors.Wrap(err, "could not hash share password")
		}
		password = string(h)
		passwordProtected = true
	}

	createdAt := &typespb.Timestamp{
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
		Mtime:             createdAt,
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

	encShare, err := utils.MarshalProtoV1ToJSON(&ps.PublicShare)
	if err != nil {
		return nil, err
	}

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	if _, ok := db[s.Id.GetOpaqueId()]; !ok {
		db[s.Id.GetOpaqueId()] = map[string]interface{}{
			"share":    string(encShare),
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
	share, err := m.GetPublicShare(ctx, u, req.Ref, false)
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
			h, err := bcrypt.GenerateFromPassword([]byte(req.Update.GetGrant().Password), m.passwordHashCost)
			if err != nil {
				return nil, errors.Wrap(err, "could not hash share password")
			}
			newPasswordEncoded = string(h)
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

	encShare, err := utils.MarshalProtoV1ToJSON(share)
	if err != nil {
		return nil, err
	}

	data, ok := db[share.Id.OpaqueId].(map[string]interface{})
	if !ok {
		data = map[string]interface{}{}
	}

	if ok && passwordChanged {
		data["password"] = newPasswordEncoded
	}
	data["share"] = string(encShare)

	db[share.Id.OpaqueId] = data

	err = m.writeDb(db)
	if err != nil {
		return nil, err
	}

	return share, nil
}

// GetPublicShare gets a public share either by ID or Token.
func (m *manager) GetPublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference, sign bool) (*link.PublicShare, error) {
	if ref.GetToken() != "" {
		ps, pw, err := m.getByToken(ctx, ref.GetToken())
		if err != nil {
			return nil, errors.New("no shares found by token")
		}
		if ps.PasswordProtected && sign {
			publicshare.AddSignature(ps, pw)
		}
		return ps, nil
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	for _, v := range db {
		d := v.(map[string]interface{})["share"]
		passDB := v.(map[string]interface{})["password"].(string)

		var ps link.PublicShare
		if err := utils.UnmarshalJSONToProtoV1([]byte(d.(string)), &ps); err != nil {
			return nil, err
		}

		if ref.GetId().GetOpaqueId() == ps.Id.OpaqueId {
			if !notExpired(&ps) {
				if err := m.revokeExpiredPublicShare(ctx, &ps, u); err != nil {
					return nil, err
				}
				return nil, errors.New("no shares found by id:" + ref.GetId().String())
			}
			if ps.PasswordProtected && sign {
				publicshare.AddSignature(&ps, passDB)
			}
			return &ps, nil
		}

	}
	return nil, errors.New("no shares found by id:" + ref.GetId().String())
}

// ListPublicShares retrieves all the shares on the manager that are valid.
func (m *manager) ListPublicShares(ctx context.Context, u *user.User, filters []*link.ListPublicSharesRequest_Filter, md *provider.ResourceInfo, sign bool) ([]*link.PublicShare, error) {
	var shares []*link.PublicShare

	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	for _, v := range db {
		var local publicShare
		if err := utils.UnmarshalJSONToProtoV1([]byte(v.(map[string]interface{})["share"].(string)), &local.PublicShare); err != nil {
			return nil, err
		}

		// skip if the share isn't created by the current user.
		if local.Creator.GetOpaqueId() != u.Id.OpaqueId || (local.Creator.GetIdp() != "" && u.Id.Idp != local.Creator.GetIdp()) {
			continue
		}

		if local.PublicShare.PasswordProtected && sign {
			publicshare.AddSignature(&local.PublicShare, local.Password)
		}

		if len(filters) == 0 {
			shares = append(shares, &local.PublicShare)
		} else {
			for i := range filters {
				if filters[i].Type == link.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID {
					if local.ResourceId.StorageId == filters[i].GetResourceId().StorageId && local.ResourceId.OpaqueId == filters[i].GetResourceId().OpaqueId {
						if notExpired(&local.PublicShare) {
							shares = append(shares, &local.PublicShare)
						} else if err := m.revokeExpiredPublicShare(ctx, &local.PublicShare, u); err != nil {
							return nil, err
						}
					}

				}
			}
		}
	}

	return shares, nil
}

// notExpired tests whether a public share is expired
func notExpired(s *link.PublicShare) bool {
	t := time.Unix(int64(s.Expiration.GetSeconds()), int64(s.Expiration.GetNanos()))
	if (s.Expiration != nil && t.After(time.Now())) || s.Expiration == nil {
		return true
	}
	return false
}

func (m *manager) cleanupExpiredShares() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	db, _ := m.readDb()

	for _, v := range db {
		d := v.(map[string]interface{})["share"]

		var ps link.PublicShare
		_ = utils.UnmarshalJSONToProtoV1([]byte(d.(string)), &ps)

		if !notExpired(&ps) {
			_ = m.revokeExpiredPublicShare(context.Background(), &ps, nil)
		}
	}
}

func (m *manager) revokeExpiredPublicShare(ctx context.Context, s *link.PublicShare, u *user.User) error {
	if !m.enableExpiredSharesCleanup {
		return nil
	}

	m.mutex.Unlock()
	defer m.mutex.Lock()

	span := trace.FromContext(ctx)
	span.AddAttributes(
		trace.StringAttribute("operation", "delete expired share"),
		trace.StringAttribute("opaqueId", s.Id.OpaqueId),
	)

	err := m.RevokePublicShare(ctx, u, &link.PublicShareReference{
		Spec: &link.PublicShareReference_Id{
			Id: &link.PublicShareId{
				OpaqueId: s.Id.OpaqueId,
			},
		},
	})
	if err != nil {
		log.Err(err).Msg(fmt.Sprintf("publicShareJSONManager: error deleting public share with opaqueId: %s", s.Id.OpaqueId))
		return err
	}

	return nil
}

// RevokePublicShare undocumented.
func (m *manager) RevokePublicShare(ctx context.Context, u *user.User, ref *link.PublicShareReference) error {
	m.mutex.Lock()
	db, err := m.readDb()
	if err != nil {
		return err
	}
	m.mutex.Unlock()

	switch {
	case ref.GetId() != nil && ref.GetId().OpaqueId != "":
		if _, ok := db[ref.GetId().OpaqueId]; ok {
			delete(db, ref.GetId().OpaqueId)
		} else {
			return errors.New("reference does not exist")
		}
	case ref.GetToken() != "":
		share, _, err := m.getByToken(ctx, ref.GetToken())
		if err != nil {
			return err
		}
		delete(db, share.Id.OpaqueId)
	default:
		return errors.New("reference does not exist")
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.writeDb(db)
}

func (m *manager) getByToken(ctx context.Context, token string) (*link.PublicShare, string, error) {
	db, err := m.readDb()
	if err != nil {
		return nil, "", err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, v := range db {
		var local link.PublicShare
		if err := utils.UnmarshalJSONToProtoV1([]byte(v.(map[string]interface{})["share"].(string)), &local); err != nil {
			return nil, "", err
		}

		if local.Token == token {
			passDB := v.(map[string]interface{})["password"].(string)
			return &local, passDB, nil
		}
	}

	return nil, "", fmt.Errorf("share with token: `%v` not found", token)
}

// GetPublicShareByToken gets a public share by its opaque token.
func (m *manager) GetPublicShareByToken(ctx context.Context, token string, auth *link.PublicShareAuthentication, sign bool) (*link.PublicShare, error) {
	db, err := m.readDb()
	if err != nil {
		return nil, err
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, v := range db {
		passDB := v.(map[string]interface{})["password"].(string)
		var local link.PublicShare
		if err := utils.UnmarshalJSONToProtoV1([]byte(v.(map[string]interface{})["share"].(string)), &local); err != nil {
			return nil, err
		}

		if local.Token == token {
			if !notExpired(&local) {
				// TODO user is not needed at all in this API.
				if err := m.revokeExpiredPublicShare(ctx, &local, nil); err != nil {
					return nil, err
				}
				break
			}

			if local.PasswordProtected {
				if authenticate(&local, passDB, auth) {
					if sign {
						publicshare.AddSignature(&local, passDB)
					}
					return &local, nil
				}

				return nil, errtypes.InvalidCredentials("json: invalid password")
			}
			return &local, nil
		}
	}

	return nil, errtypes.NotFound(fmt.Sprintf("share with token: `%v` not found", token))
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

func authenticate(share *link.PublicShare, pw string, auth *link.PublicShareAuthentication) bool {
	switch {
	case auth.GetPassword() != "":
		if err := bcrypt.CompareHashAndPassword([]byte(pw), []byte(auth.GetPassword())); err == nil {
			return true
		}
	case auth.GetSignature() != nil:
		sig := auth.GetSignature()
		now := time.Now()
		expiration := time.Unix(int64(sig.GetSignatureExpiration().GetSeconds()), int64(sig.GetSignatureExpiration().GetNanos()))
		if now.After(expiration) {
			return false
		}
		s := publicshare.CreateSignature(share.Token, pw, expiration)
		return sig.GetSignature() == s
	}
	return false
}

type publicShare struct {
	link.PublicShare
	Password string `json:"password"`
}
