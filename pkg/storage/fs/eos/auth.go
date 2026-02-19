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

package eos

import (
	"context"
	"path"
	"strconv"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/storage/utils/acl"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

func (fs *Eosfs) extractUIDAndGID(u *userpb.User) (eosclient.Authorization, error) {
	if u.UidNumber == 0 {
		return eosclient.Authorization{}, errors.New("eosfs: uid missing for user")
	}
	if u.GidNumber == 0 {
		return eosclient.Authorization{}, errors.New("eosfs: gid missing for user")
	}
	return eosclient.Authorization{Role: eosclient.Role{UID: strconv.FormatInt(u.UidNumber, 10), GID: strconv.FormatInt(u.GidNumber, 10)}}, nil
}

func (fs *Eosfs) getUIDGateway(ctx context.Context, u *userpb.UserId) (eosclient.Authorization, error) {
	log := appctx.GetLogger(ctx)
	if userIDInterface, err := fs.userIDCache.Get(u.OpaqueId); err == nil {
		log.Debug().Msg("eosfs: found cached user " + u.OpaqueId)
		return fs.extractUIDAndGID(userIDInterface.(*userpb.User))
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return eosclient.Authorization{}, errors.Wrap(err, "eosfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId:                 u,
		SkipFetchingUserGroups: true,
	})
	if err != nil {
		_ = fs.userIDCache.SetWithTTL(u.OpaqueId, &userpb.User{}, 12*time.Hour)
		return eosclient.Authorization{}, errors.Wrap(err, "eosfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		_ = fs.userIDCache.SetWithTTL(u.OpaqueId, &userpb.User{}, 12*time.Hour)
		return eosclient.Authorization{}, status.NewErrorFromCode(getUserResp.Status.Code, "eosfs")
	}

	_ = fs.userIDCache.Set(u.OpaqueId, getUserResp.User)
	return fs.extractUIDAndGID(getUserResp.User)
}

func (fs *Eosfs) getUserIDGateway(ctx context.Context, uid string) (*userpb.UserId, error) {
	log := appctx.GetLogger(ctx)
	// Handle the case of root
	if uid == "0" {
		return nil, errtypes.BadRequest("eosfs: cannot return root user")
	}

	if userIDInterface, err := fs.userIDCache.Get(uid); err == nil {
		log.Debug().Msg("eosfs: found cached uid " + uid)
		return userIDInterface.(*userpb.UserId), nil
	}

	log.Debug().Msg("eosfs: retrieving user from gateway for uid " + uid)
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error getting gateway grpc client")
	}
	getUserResp, err := client.GetUserByClaim(ctx, &userpb.GetUserByClaimRequest{
		Claim:                  "uid",
		Value:                  uid,
		SkipFetchingUserGroups: true,
	})
	if err != nil {
		// Insert an empty object in the cache so that we don't make another call
		// for a specific amount of time
		_ = fs.userIDCache.SetWithTTL(uid, &userpb.UserId{}, 12*time.Hour)
		return nil, errors.Wrap(err, "eosfs: error getting user")
	}
	if getUserResp.Status.Code != rpc.Code_CODE_OK {
		// Insert an empty object in the cache so that we don't make another call
		// for a specific amount of time
		_ = fs.userIDCache.SetWithTTL(uid, &userpb.UserId{}, 12*time.Hour)
		return nil, status.NewErrorFromCode(getUserResp.Status.Code, "eosfs")
	}

	_ = fs.userIDCache.Set(uid, getUserResp.User.Id)
	return getUserResp.User.Id, nil
}

func (fs *Eosfs) getUserAuth(ctx context.Context, u *userpb.User, fn string) (eosclient.Authorization, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserAuth.Role.UID != "" && fs.singleUserAuth.Role.GID != "" {
			return fs.singleUserAuth, nil
		}
		var err error
		fs.singleUserAuth, err = fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		return fs.singleUserAuth, err
	}

	if utils.IsLightweightUser(u) {
		return fs.getEOSToken(ctx, u, fn)
	}

	return fs.extractUIDAndGID(u)
}

// Generate an EOS token that acts on behalf of the owner of the file or folder `fn`
func (fs *Eosfs) getEOSToken(ctx context.Context, u *userpb.User, fn string) (eosclient.Authorization, error) {
	if fn == "" {
		return eosclient.Authorization{}, errtypes.BadRequest("eosfs: path cannot be empty")
	}

	daemonAuth, _ := fs.getDaemonAuth(ctx)
	info, err := fs.c.GetFileInfoByPath(ctx, daemonAuth, fn)
	if err != nil {
		return eosclient.Authorization{}, err
	}
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		},
	}

	// For files, the "x" bit cannot be set, so we initialize without
	// and if it's a directory, we add `x` later
	perm := "rw"
	for _, e := range info.SysACL.Entries {
		if e.Type == acl.TypeLightweight && e.Qualifier == u.Id.OpaqueId {
			perm = e.Permissions
			break
		}
	}

	p := path.Clean(fn)
	for p != "." && p != fs.conf.Namespace {
		key := p + "!" + perm
		if tknIf, err := fs.tokenCache.Get(key); err == nil {
			return eosclient.Authorization{Token: tknIf.(string)}, nil
		}
		p = path.Dir(p)
	}

	if info.IsDir {
		// EOS expects directories to have a trailing slash when generating tokens
		fn = path.Clean(fn) + "/"
		perm = perm + "x"
	}
	tkn, err := fs.c.GenerateToken(ctx, auth, fn, &acl.Entry{Permissions: perm})
	if err != nil {
		return eosclient.Authorization{}, err
	}

	key := path.Clean(fn) + "!" + perm
	_ = fs.tokenCache.SetWithExpire(key, tkn, time.Second*time.Duration(fs.conf.TokenExpiry))

	return eosclient.Authorization{Token: tkn}, nil
}

// // Returns an Authorization wich assumes the role of the owner of the file `fn`
func (fs *Eosfs) getOwnerAuth(ctx context.Context, fn string) (eosclient.Authorization, error) {
	if fn == "" {
		return eosclient.Authorization{}, errtypes.BadRequest("eosfs: path cannot be empty")
	}

	daemonAuth, _ := fs.getDaemonAuth(ctx)
	info, err := fs.c.GetFileInfoByPath(ctx, daemonAuth, fn)
	if err != nil {
		return eosclient.Authorization{}, err
	}
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		},
	}

	return auth, nil
}

// Returns an eosclient.Authorization object with the uid/gid of the daemon user
// This is a system user with read-only access to files.
// We use it e.g. when retrieving metadata from a file when accessing through a guest account,
// so we can look up which user to impersonate (i.e. the owner)
func (fs *Eosfs) getDaemonAuth(ctx context.Context) (eosclient.Authorization, error) {
	if fs.conf.ForceSingleUserMode {
		if fs.singleUserAuth.Role.UID != "" && fs.singleUserAuth.Role.GID != "" {
			return fs.singleUserAuth, nil
		}
		var err error
		fs.singleUserAuth, err = fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		return fs.singleUserAuth, err
	}
	return utils.GetDaemonAuth(), nil
}
