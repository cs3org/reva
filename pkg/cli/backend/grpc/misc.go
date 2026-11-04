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

package grpc

import (
	"context"
	"time"

	authapp "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	preferences "github.com/cs3org/go-cs3apis/cs3/preferences/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
)

type spaces struct{ b *Backend }

func (s spaces) List(ctx context.Context) ([]*provider.StorageSpace, error) {
	return nil, notAvailable("the space list")
}

func (s spaces) Quota(ctx context.Context, ref *provider.Reference) (*backend.Quota, error) {
	res, err := call(ctx, s.b, gateway.GatewayAPIClient.GetQuota, &gateway.GetQuotaRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	q := &backend.Quota{Total: res.TotalBytes, Used: res.UsedBytes}
	if q.Total > q.Used {
		q.Remaining = q.Total - q.Used
	}
	return q, nil
}

type trash struct{ b *Backend }

// home returns ref, or the home of the caller for a nil ref, because the
// recycle calls need a space to act on.
func (t trash) home(ctx context.Context, c gateway.GatewayAPIClient, ref *provider.Reference) (*provider.Reference, error) {
	if ref != nil {
		return ref, nil
	}
	res, err := check(c.GetHome(ctx, &provider.GetHomeRequest{}))
	if err != nil {
		return nil, err
	}
	return &provider.Reference{Path: res.Path}, nil
}

func timestamp(t time.Time) *types.Timestamp {
	if t.IsZero() {
		return nil
	}
	return &types.Timestamp{Seconds: uint64(t.Unix())}
}

func (t trash) List(ctx context.Context, ref *provider.Reference, from, to time.Time) ([]*provider.RecycleItem, error) {
	c, ctx, err := t.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	if ref, err = t.home(ctx, c, ref); err != nil {
		return nil, err
	}
	res, err := check(c.ListRecycle(ctx, &provider.ListRecycleRequest{Ref: ref, FromTs: timestamp(from), ToTs: timestamp(to)}))
	if err != nil {
		return nil, err
	}
	return res.RecycleItems, nil
}

func (t trash) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	c, ctx, err := t.b.gw(ctx)
	if err != nil {
		return err
	}
	if ref, err = t.home(ctx, c, ref); err != nil {
		return err
	}
	_, err = check(c.RestoreRecycleItem(ctx, &provider.RestoreRecycleItemRequest{Ref: ref, Key: key}))
	return err
}

func (t trash) Purge(ctx context.Context, ref *provider.Reference, key string) error {
	c, ctx, err := t.b.gw(ctx)
	if err != nil {
		return err
	}
	if ref, err = t.home(ctx, c, ref); err != nil {
		return err
	}
	_, err = check(c.PurgeRecycle(ctx, &provider.PurgeRecycleRequest{Ref: ref, Key: key}))
	return err
}

type versions struct{ b *Backend }

func (v versions) List(ctx context.Context, ref *provider.Reference) ([]*provider.FileVersion, error) {
	res, err := call(ctx, v.b, gateway.GatewayAPIClient.ListFileVersions, &provider.ListFileVersionsRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	return res.Versions, nil
}

func (v versions) Restore(ctx context.Context, ref *provider.Reference, key string) error {
	_, err := call(ctx, v.b, gateway.GatewayAPIClient.RestoreFileVersion, &provider.RestoreFileVersionRequest{Ref: ref, Key: key})
	return err
}

type locks struct{ b *Backend }

func (l locks) Get(ctx context.Context, ref *provider.Reference) (*provider.Lock, error) {
	res, err := call(ctx, l.b, gateway.GatewayAPIClient.GetLock, &provider.GetLockRequest{Ref: ref})
	if err != nil {
		return nil, err
	}
	return res.Lock, nil
}

func (l locks) Set(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	_, err := call(ctx, l.b, gateway.GatewayAPIClient.SetLock, &provider.SetLockRequest{Ref: ref, Lock: lock})
	return err
}

func (l locks) Refresh(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	_, err := call(ctx, l.b, gateway.GatewayAPIClient.RefreshLock, &provider.RefreshLockRequest{Ref: ref, Lock: lock})
	return err
}

func (l locks) Unlock(ctx context.Context, ref *provider.Reference, lock *provider.Lock) error {
	_, err := call(ctx, l.b, gateway.GatewayAPIClient.Unlock, &provider.UnlockRequest{Ref: ref, Lock: lock})
	return err
}

type tokens struct{ b *Backend }

func (t tokens) List(ctx context.Context) ([]*authapp.AppPassword, error) {
	res, err := call(ctx, t.b, gateway.GatewayAPIClient.ListAppPasswords, &authapp.ListAppPasswordsRequest{})
	if err != nil {
		return nil, err
	}
	return res.AppPasswords, nil
}

func (t tokens) Create(ctx context.Context, req *authapp.GenerateAppPasswordRequest) (*authapp.AppPassword, error) {
	res, err := call(ctx, t.b, gateway.GatewayAPIClient.GenerateAppPassword, req)
	if err != nil {
		return nil, err
	}
	return res.AppPassword, nil
}

func (t tokens) Revoke(ctx context.Context, id string) error {
	_, err := call(ctx, t.b, gateway.GatewayAPIClient.InvalidateAppPassword, &authapp.InvalidateAppPasswordRequest{Password: id})
	return err
}

type users struct{ b *Backend }

func (u users) Me(ctx context.Context) (*userpb.User, error) {
	c, ctx, err := u.b.gw(ctx)
	if err != nil {
		return nil, err
	}
	token, _ := appctx.ContextGetToken(ctx)
	res, err := check(c.WhoAmI(ctx, &gateway.WhoAmIRequest{Token: token}))
	if err != nil {
		return nil, err
	}
	return res.User, nil
}

func (u users) ByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	res, err := call(ctx, u.b, gateway.GatewayAPIClient.GetUserByClaim, &userpb.GetUserByClaimRequest{Claim: claim, Value: value})
	if err != nil {
		return nil, err
	}
	return res.User, nil
}

func (b *Backend) GetPreference(ctx context.Context, namespace, key string) (string, error) {
	res, err := call(ctx, b, gateway.GatewayAPIClient.GetKey, &preferences.GetKeyRequest{Key: &preferences.PreferenceKey{Namespace: namespace, Key: key}})
	if err != nil {
		return "", err
	}
	return res.Val, nil
}

func (b *Backend) SetPreference(ctx context.Context, namespace, key, value string) error {
	_, err := call(ctx, b, gateway.GatewayAPIClient.SetKey, &preferences.SetKeyRequest{Key: &preferences.PreferenceKey{Namespace: namespace, Key: key}, Val: value})
	return err
}
