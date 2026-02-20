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
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	eosclient "github.com/cs3org/reva/v3/pkg/storage/fs/eos/client"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/pkg/errors"
)

// GetLock returns an existing lock on the given reference.
func (fs *Eosfs) GetLock(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error resolving reference")
	}
	user, err := utils.GetUser(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: no user in ctx")
	}

	// the cs3apis require to have the read permission on the resource
	// to get the eventual lock.
	has, err := fs.userHasReadAccess(ctx, user, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error checking read access to resource")
	}
	if !has {
		return nil, errtypes.BadRequest("user has no read access on resource")
	}

	//path = fs.wrap(ctx, path)
	return fs.getLock(ctx, user, path, ref)
}

// SetLock puts a lock on the given reference.
func (fs *Eosfs) SetLock(ctx context.Context, ref *provider.Reference, l *provider.Lock) error {
	if l.Type == provider.LockType_LOCK_TYPE_SHARED {
		return errtypes.NotSupported("shared lock not yet implemented")
	}

	oldLock, err := fs.GetLock(ctx, ref)
	if err == nil && oldLock.LockId != "" {
		return errtypes.Conflict("file is already locked, lockId: " + oldLock.LockId)
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: no user in ctx")
	}

	// the cs3apis require to have the write permission on the resource
	// to set a lock. because in eos we can set attrs even if the user does
	// not have the write permission, we need to check if the user that made
	// the request has it
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("eosfs: cannot check if user %s has write access on resource", user.Username))
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	// the user in the lock could differ from the user in the context
	// in that case, also the user in the lock MUST have the write permission
	if l.User != nil && !utils.UserEqual(user.Id, l.User) {
		has, err := fs.userIDHasWriteAccess(ctx, l.User, ref)
		if err != nil {
			return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
		}
		if !has {
			return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
		}
	}

	//path = fs.wrap(ctx, path)
	return fs.setLock(ctx, l, path)
}

// RefreshLock refreshes an existing lock on the given reference.
func (fs *Eosfs) RefreshLock(ctx context.Context, ref *provider.Reference, newLock *provider.Lock, existingLockID string) error {
	if newLock.Type == provider.LockType_LOCK_TYPE_SHARED {
		return errtypes.NotSupported("shared lock not yet implemented")
	}

	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: error getting user")
	}

	// check if the holder is the same of the new lock
	if !sameHolder(oldLock, newLock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	if existingLockID != "" && oldLock.LockId != existingLockID {
		return errtypes.BadRequest("mismatched existing lockId: " + existingLockID)
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	//path = fs.wrap(ctx, path)

	// the cs3apis require to have the write permission on the resource
	// to set a lock
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	return fs.setLock(ctx, newLock, path)
}

// Unlock removes an existing lock from the given reference.
func (fs *Eosfs) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	oldLock, err := fs.GetLock(ctx, ref)
	if err != nil {
		switch err.(type) {
		case errtypes.NotFound:
			// the lock does not exist
			return errtypes.BadRequest("file was not locked")
		default:
			return err
		}
	}

	// check if the lock id of the lock corresponds to the stored lock
	if oldLock.LockId != lock.LockId {
		return errtypes.BadRequest("lock id does not match")
	}

	if !sameHolder(oldLock, lock) {
		return errtypes.BadRequest("caller does not hold the lock")
	}

	user, err := utils.GetUser(ctx)
	if err != nil {
		return errors.Wrap(err, "eosfs: error getting user")
	}

	// the cs3apis require to have the write permission on the resource
	// to remove the lock
	has, err := fs.userHasWriteAccess(ctx, user, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: cannot check if user has write access on resource")
	}
	if !has {
		return errtypes.PermissionDenied(fmt.Sprintf("user %s has no write access on resource", user.Username))
	}

	path, err := fs.resolve(ctx, ref)
	if err != nil {
		return errors.Wrap(err, "eosfs: error resolving reference")
	}
	//path = fs.wrap(ctx, path)

	return fs.removeLockAttrs(ctx, path, fs.EncodeAppName(lock.AppName))
}

// EncodeAppName returns the string to be used as EOS "app" tag, both in uploads and when handling locks.
// Note that the GET (and PUT) operations in eosbinary.go and eoshttp.go use a `read`
// (resp. `write`) app name when no locks are involved.
func (fs *Eosfs) EncodeAppName(a string) string {
	r := strings.NewReplacer(" ", "_")
	return eosclient.EosAppPrefix + "_" + strings.ToLower(r.Replace(a))
}

func (fs *Eosfs) getLockPayloads(ctx context.Context, path string) (string, string, error) {
	// sys attributes want root auth, buddy
	sysAuth := getSystemAuth()

	data, err := fs.c.GetAttr(ctx, sysAuth, "sys."+lockPayloadKey, path)
	if err != nil {
		return "", "", err
	}

	eoslock, err := fs.c.GetAttr(ctx, sysAuth, "sys."+eosLockKey, path)
	if err != nil {
		return "", "", err
	}

	return data.Val, eoslock.Val, nil
}

func (fs *Eosfs) removeLockAttrs(ctx context.Context, path, app string) error {
	sysAuth := getSystemAuth()

	err := fs.c.UnsetAttr(ctx, sysAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  eosLockKey,
	}, false, path, app)
	if err != nil {
		return errors.Wrap(err, "eosfs: error unsetting the eos lock")
	}

	err = fs.c.UnsetAttr(ctx, sysAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  lockPayloadKey,
	}, false, path, app)
	if err != nil {
		return errors.Wrap(err, "eosfs: error unsetting the lock payload")
	}

	return nil
}

func (fs *Eosfs) getLock(ctx context.Context, user *userpb.User, path string, ref *provider.Reference) (*provider.Lock, error) {
	// the cs3apis require to have the read permission on the resource
	// to get the eventual lock.
	has, err := fs.userHasReadAccess(ctx, user, ref)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: error checking read access to resource")
	}
	if !has {
		return nil, errtypes.BadRequest("user has not read access on resource")
	}

	d, eosl, err := fs.getLockPayloads(ctx, path)
	if err != nil {
		if !errors.Is(err, eosclient.AttrNotExistsError) {
			return nil, errtypes.NotFound("lock not found for ref")
		}
	}

	l, err := decodeLock(d, eosl)
	if err != nil {
		return nil, errors.Wrap(err, "eosfs: malformed lock payload")
	}

	if time.Unix(int64(l.Expiration.Seconds), 0).Before(time.Now()) {
		// the lock expired
		if err := fs.removeLockAttrs(ctx, path, fs.EncodeAppName(l.AppName)); err != nil {
			return nil, err
		}
		return nil, errtypes.NotFound("lock not found for ref")
	}

	return l, nil
}

func (fs *Eosfs) setLock(ctx context.Context, lock *provider.Lock, path string) error {
	sysAuth := getSystemAuth()

	encodedLock, eosLock, err := fs.encodeLock(lock)
	if err != nil {
		return errors.Wrap(err, "eosfs: error encoding lock")
	}

	// set eos lock
	err = fs.c.SetAttr(ctx, sysAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  eosLockKey,
		Val:  eosLock,
	}, false, false, path, fs.EncodeAppName(lock.AppName))
	switch {
	case errors.Is(err, eosclient.FileIsLockedError):
		return errtypes.Conflict("resource already locked")
	case err != nil:
		return errors.Wrap(err, "eosfs: error setting eos lock")
	}

	// set payload
	err = fs.c.SetAttr(ctx, sysAuth, &eosclient.Attribute{
		Type: SystemAttr,
		Key:  lockPayloadKey,
		Val:  encodedLock,
	}, false, false, path, fs.EncodeAppName(lock.AppName))
	if err != nil {
		return errors.Wrap(err, "eosfs: error setting lock payload")
	}
	return nil
}

func (fs *Eosfs) getUserFromID(ctx context.Context, userID *userpb.UserId) (*userpb.User, error) {
	client, err := pool.GetGatewayServiceClient(pool.Endpoint(fs.conf.GatewaySvc))
	if err != nil {
		return nil, err
	}
	res, err := client.GetUser(ctx, &userpb.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errtypes.InternalError(res.Status.Message)
	}
	return res.User, nil
}

func (fs *Eosfs) userHasWriteAccess(ctx context.Context, user *userpb.User, ref *provider.Reference) (bool, error) {
	ctx = appctx.ContextSetUser(ctx, user)
	resInfo, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return false, err
	}
	return resInfo.PermissionSet.InitiateFileUpload, nil
}

func (fs *Eosfs) userIDHasWriteAccess(ctx context.Context, userID *userpb.UserId, ref *provider.Reference) (bool, error) {
	user, err := fs.getUserFromID(ctx, userID)
	if err != nil {
		return false, nil
	}
	return fs.userHasWriteAccess(ctx, user, ref)
}

func (fs *Eosfs) userHasReadAccess(ctx context.Context, user *userpb.User, ref *provider.Reference) (bool, error) {
	ctx = appctx.ContextSetUser(ctx, user)
	resInfo, err := fs.GetMD(ctx, ref, nil)
	if err != nil {
		return false, err
	}
	return resInfo.PermissionSet.InitiateFileDownload, nil
}

func (fs *Eosfs) encodeLock(l *provider.Lock) (string, string, error) {
	data, err := json.Marshal(l)
	if err != nil {
		return "", "", err
	}
	var a string
	if l.AppName != "" {
		// cf. upload implementation
		a = fs.EncodeAppName(l.AppName)
	} else {
		a = "*"
	}
	var u string
	if l.User != nil {
		u = l.User.OpaqueId
	} else {
		u = "*"
	}
	// the eos lock has hardcoded type "shared" because that's what eos supports. This is good enough
	// for apps via WOPI and for checkout/checkin behavior, not for "exclusive" (no read access unless holding the lock).
	return b64.StdEncoding.EncodeToString(data),
		fmt.Sprintf("expires:%d,type:shared,owner:%s:%s", l.Expiration.Seconds, u, a),
		nil
}

func decodeLock(content string, eosLock string) (*provider.Lock, error) {
	d, err := b64.StdEncoding.DecodeString(content)
	if err != nil {
		return nil, err
	}

	l := new(provider.Lock)
	err = json.Unmarshal(d, l)
	if err != nil {
		return nil, err
	}

	// validate that the eosLock respect the format, otherwise raise error
	if !eosLockReg.MatchString(eosLock) {
		return nil, errtypes.BadRequest("eos lock payload does not match expected format: " + eosLock)
	}

	return l, nil
}

func sameHolder(l1, l2 *provider.Lock) bool {
	same := true
	if l1.User != nil || l2.User != nil {
		same = utils.UserEqual(l1.User, l2.User)
	}
	if l1.AppName != "" || l2.AppName != "" {
		same = l1.AppName == l2.AppName
	}
	return same
}
