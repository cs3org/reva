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

package gateway

import (
	"context"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/permissions"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
)

// ensureLightweightHome makes sure the lightweight account u has a home: the
// folder at lightweight_home_layout, shared with it by lightweight_home_owner.
// The folder is provisioned by the create_lightweight_home_hook of the storage
// provider that holds it.
//
// ctx must not carry u's token: machine auth for the owner goes through
// GetUserByClaim, which the lightweight scope does not allow.
func (s *svc) ensureLightweightHome(ctx context.Context, u *userpb.User, token string) error {
	if _, err := s.createHomeCache.Get(u.Id.OpaqueId); err == nil {
		return nil
	}

	home := templates.WithUser(u, s.c.LightweightHomeLayout)
	userCtx := withAuth(ctx, u, token)

	// A lightweight account can stat any path, but only gets permissions
	// on the ones shared with it.
	statRes, err := s.Stat(userCtx, &provider.StatRequest{Ref: &provider.Reference{Path: home}})
	if err != nil {
		return errors.Wrap(err, "error statting lightweight home")
	}
	switch {
	case statRes.Status.Code == rpc.Code_CODE_OK && statRes.Info.GetPermissionSet().GetStat():
		s.markHomeCreated(u)
		return nil
	case statRes.Status.Code == rpc.Code_CODE_OK:
		// The folder exists but is not shared yet, e.g. because a previous
		// login failed after provisioning it.
	case statRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		c, err := s.findByPath(userCtx, home)
		if err != nil {
			return errors.Wrap(err, "error finding storage provider for lightweight home")
		}
		createRes, err := c.CreateHome(userCtx, &provider.CreateHomeRequest{})
		if err != nil {
			return errors.Wrap(err, "error calling CreateHome")
		}
		if createRes.Status.Code != rpc.Code_CODE_OK {
			return status.NewErrorFromCode(createRes.Status.Code, "gateway")
		}
	default:
		return status.NewErrorFromCode(statRes.Status.Code, "gateway")
	}

	ownerCtx, err := s.impersonate(ctx, s.c.LightweightHomeOwner)
	if err != nil {
		return errors.Wrap(err, "error impersonating lightweight home owner")
	}
	ownerStatRes, err := s.Stat(ownerCtx, &provider.StatRequest{Ref: &provider.Reference{Path: home}})
	if err != nil {
		return errors.Wrap(err, "error statting lightweight home as its owner")
	}
	if ownerStatRes.Status.Code != rpc.Code_CODE_OK {
		return errors.Wrapf(status.NewErrorFromCode(ownerStatRes.Status.Code, "gateway"), "lightweight home %s not available to %s", home, s.c.LightweightHomeOwner)
	}

	shareRes, err := s.CreateShare(ownerCtx, &collaboration.CreateShareRequest{
		ResourceInfo: ownerStatRes.Info,
		Grant: &collaboration.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   &provider.Grantee_UserId{UserId: u.Id},
			},
			Permissions: &collaboration.SharePermissions{
				Permissions: permissions.NewEditorRole().CS3ResourcePermissions(),
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "error sharing lightweight home")
	}
	if shareRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewErrorFromCode(shareRes.Status.Code, "gateway")
	}

	appctx.GetLogger(ctx).Info().Str("home", home).Str("user", u.Username).Msg("shared lightweight home")
	s.markHomeCreated(u)
	return nil
}

func (s *svc) markHomeCreated(u *userpb.User) {
	if s.c.CreateHomeCacheTTL > 0 {
		_ = s.createHomeCache.Set(u.Id.OpaqueId, true)
	}
}

// impersonate returns a context authenticated as username via machine auth.
func (s *svc) impersonate(ctx context.Context, username string) (context.Context, error) {
	authRes, err := s.Authenticate(ctx, &gateway.AuthenticateRequest{
		Type:         "machine",
		ClientId:     username,
		ClientSecret: s.c.MachineSecret,
	})
	if err != nil {
		return nil, err
	}
	if authRes.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.New("error impersonating user: " + authRes.Status.Message)
	}
	return withAuth(ctx, authRes.User, authRes.Token), nil
}

func withAuth(ctx context.Context, u *userpb.User, token string) context.Context {
	ctx = appctx.ContextSetToken(ctx, token)
	ctx = appctx.ContextSetUser(ctx, u)
	return metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, token)
}
