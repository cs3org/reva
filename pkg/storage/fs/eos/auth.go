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
	"fmt"
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

func extractUIDAndGID(u *userpb.User) (eosclient.Authorization, error) {
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
		return extractUIDAndGID(userIDInterface.(*userpb.User))
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
	return extractUIDAndGID(getUserResp.User)
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

// Return an auth object with uid and gid set to the user passed.
// This will error on lightweight users
func (fs *Eosfs) getUserAuth(ctx context.Context) (eosclient.Authorization, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return invalidAuth(), fmt.Errorf("eosfs: no user found in context")
	}

	if fs.conf.ForceSingleUserMode {
		if fs.singleUserAuth.Role.UID != "" && fs.singleUserAuth.Role.GID != "" {
			return fs.singleUserAuth, nil
		}
		var err error
		fs.singleUserAuth, err = fs.getUIDGateway(ctx, &userpb.UserId{OpaqueId: fs.conf.SingleUsername})
		return fs.singleUserAuth, err
	}

	if utils.IsLightweightUser(u) {
		return invalidAuth(), fmt.Errorf("eosfs: cannot get uid/gid for external user")
		//return fs.getEOSToken(ctx, u, fn)
	}

	return extractUIDAndGID(u)
}

// Return an auth object with uid and gid set to the user passed
// if it is a primary user. Otherwise no uid/gid are set,
// but a token is set instead, valid for the path fn that is passed
// This will error on lightweight users
func (fs *Eosfs) getUserAuthOrToken(ctx context.Context, fn string) (eosclient.Authorization, error) {
	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return invalidAuth(), fmt.Errorf("eosfs: no user found in context")
	}

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

	return extractUIDAndGID(u)
}

// TODO: return nobody or some blocked user without access
// because empty defaults to the system user (sudo'er)
func invalidAuth() eosclient.Authorization {
	return eosclient.Authorization{}
}

// Generate an EOS token that acts on behalf of the owner of the file or folder `path`
func (fs *Eosfs) getEOSToken(ctx context.Context, u *userpb.User, fn string) (eosclient.Authorization, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Msgf("Fetching EOS token for user %s for path %s", u.Id.OpaqueId, fn)
	if fn == "" {
		return invalidAuth(), errtypes.BadRequest("eosfs: path cannot be empty when generating a token")
	}

	// Let's check the token cache first, before doing expensive operations
	// For the cache, we also check if there are tokens on higher-level resources
	// which would also give us access to this resource
	// p := path.Clean(fn)
	// for p != "." && p != fs.conf.Namespace {
	// 	cacheKey := p + "!" + u.Id.OpaqueId
	// 	if tknIf, err := fs.tokenCache.Get(cacheKey); err == nil {
	// 		return eosclient.Authorization{Token: tknIf.(string)}, nil
	// 	}
	// 	p = path.Dir(p)
	// }
	// log.Info().Msgf("EOS token after loop", u.Id.OpaqueId, fn)

	daemonAuth, err := fs.getDaemonAuth(ctx)
	if err != nil {
		return invalidAuth(), err
	}

	// TODO: should check token cache first
	info, err := fs.c.GetFileInfoByPath(ctx, daemonAuth, fn)
	if err != nil {
		return invalidAuth(), err
	}

	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		},
	}

	// For files, the "x" bit cannot be set, so we initialize without
	// and if it's a directory, we add `x` later
	// TODO: why default to rw? should be empty!
	perm := "rw"
	for _, e := range info.SysACL.Entries {
		if e.Type == acl.TypeLightweight && e.Qualifier == u.Id.OpaqueId {
			perm = e.Permissions
			break
		}
	}

	if info.IsDir {
		// EOS expects directories to have a trailing slash when generating tokens
		fn = path.Clean(fn) + "/"
		perm = perm + "x"
	}
	tkn, err := fs.c.GenerateToken(ctx, auth, fn, &acl.Entry{Permissions: perm})
	if err != nil {
		return invalidAuth(), err
	}

	// Set token in the cache
	cacheKey := path.Clean(fn) + "!" + u.Id.OpaqueId
	_ = fs.tokenCache.SetWithExpire(cacheKey, tkn, time.Second*time.Duration(fs.conf.TokenExpiry))

	return eosclient.Authorization{Token: tkn}, nil
}

// TODO: should check if we can also get rid of the stat here
// Returns an Authorization wich assumes the role of the owner of the file `fn`
func (fs *Eosfs) getOwnerAuth(ctx context.Context, fn string) (eosclient.Authorization, error) {
	if fn == "" {
		return invalidAuth(), errtypes.BadRequest("eosfs: path cannot be empty")
	}

	daemonAuth, _ := fs.getDaemonAuth(ctx)
	info, err := fs.c.GetFileInfoByPath(ctx, daemonAuth, fn)
	if err != nil {
		return invalidAuth(), err
	}
	auth := eosclient.Authorization{
		Role: eosclient.Role{
			UID: strconv.FormatUint(info.UID, 10),
			GID: strconv.FormatUint(info.GID, 10),
		},
	}

	return auth, nil
}

// TODO: should we make `daemon` configurable?

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
	return eosclient.Authorization{Role: eosclient.Role{UID: "2", GID: "2"}}, nil
}

// This function is used when we don't want to pass any additional auth info.
// Because we later populate the secret key for gRPC, we will be automatically
// mapped to the user which is mapped to the auth key in EOS's vid list.
// For CERNBox this comes down to the cbox user.
func getSystemAuth() eosclient.Authorization {
	return eosclient.Authorization{}
}
