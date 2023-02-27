// Copyright 2018-2023 CERN
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

package ocmshares

import (
	"context"

	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitev1beta1 "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth"
	"github.com/cs3org/reva/pkg/auth/manager/registry"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ocmshares", New)
}

type manager struct {
	c  *config
	gw gateway.GatewayAPIClient
}

type config struct {
	GatewayAddr string `mapstructure:"gateway_addr"`
}

func (c *config) init() {
	c.GatewayAddr = sharedconf.GetGatewaySVC(c.GatewayAddr)
}

func New(m map[string]interface{}) (auth.Manager, error) {
	var mgr manager
	if err := mgr.Configure(m); err != nil {
		return nil, err
	}

	return &mgr, nil
}

func (m *manager) Configure(ml map[string]interface{}) error {
	var c config
	if err := mapstructure.Decode(ml, &c); err != nil {
		return errors.Wrap(err, "error decoding config")
	}
	c.init()
	m.c = &c

	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewayAddr))
	if err != nil {
		return err
	}
	m.gw = gw

	return nil
}

func (m *manager) Authenticate(ctx context.Context, token, _ string) (*userpb.User, map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx).With().Str("token", token).Logger()
	shareRes, err := m.gw.GetOCMShareByToken(ctx, &ocm.GetOCMShareByTokenRequest{
		Token: token,
	})

	switch {
	case err != nil:
		log.Error().Err(err).Msg("error getting ocm share by token")
		return nil, nil, err
	case shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		log.Debug().Msg("ocm share not found")
		return nil, nil, errtypes.NotFound(shareRes.Status.Message)
	case shareRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
		log.Debug().Msg("permission denied")
		return nil, nil, errtypes.InvalidCredentials(shareRes.Status.Message)
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		log.Error().Interface("status", shareRes.Status).Msg("got unexpected error in the grpc call to GetOCMShare")
		return nil, nil, errtypes.InternalError(shareRes.Status.Message)
	}

	// the user authenticated using the ocmshares authentication method
	// is the recipient of the share
	u := shareRes.Share.Grantee.GetUserId()

	d, err := utils.MarshalProtoV1ToJSON(shareRes.GetShare().Creator)
	if err != nil {
		return nil, nil, err
	}

	o := &typesv1beta1.Opaque{
		Map: map[string]*typesv1beta1.OpaqueEntry{
			"user-filter": {
				Decoder: "json",
				Value:   d,
			},
		},
	}

	userRes, err := m.gw.GetAcceptedUser(ctx, &invitev1beta1.GetAcceptedUserRequest{
		RemoteUserId: u,
		Opaque:       o,
	})

	switch {
	case err != nil:
		return nil, nil, err
	case userRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(shareRes.Status.Message)
	case userRes.Status.Code != rpc.Code_CODE_OK:
		return nil, nil, errtypes.InternalError(userRes.Status.Message)
	}

	scope, err := scope.AddOCMShareScope(shareRes.Share, getRole(shareRes.Share), nil)
	if err != nil {
		return nil, nil, err
	}

	return userRes.RemoteUser, scope, nil
}

func getRole(s *ocm.Share) authpb.Role {
	// TODO: consider to somehow merge the permissions from all the access methods?
	// it's not clear infact which should be the role when webdav is editor role while
	// webapp is only view mode for example
	// this implementation considers only the simple case in which when a client creates
	// a share with multiple access methods, the permissions are matching in all of them.
	for _, m := range s.AccessMethods {
		switch v := m.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			p := v.WebdavOptions.Permissions
			if p.InitiateFileUpload {
				return authpb.Role_ROLE_EDITOR
			}
			if p.InitiateFileDownload {
				return authpb.Role_ROLE_VIEWER
			}
		case *ocm.AccessMethod_WebappOptions:
			viewMode := v.WebappOptions.ViewMode
			if viewMode == providerv1beta1.ViewMode_VIEW_MODE_VIEW_ONLY ||
				viewMode == providerv1beta1.ViewMode_VIEW_MODE_READ_ONLY ||
				viewMode == providerv1beta1.ViewMode_VIEW_MODE_PREVIEW {
				return authpb.Role_ROLE_VIEWER
			}
			if viewMode == providerv1beta1.ViewMode_VIEW_MODE_READ_WRITE {
				return authpb.Role_ROLE_EDITOR
			}
		}
	}
	return authpb.Role_ROLE_INVALID
}
