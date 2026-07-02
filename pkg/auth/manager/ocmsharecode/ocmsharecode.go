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

// Package ocmsharecode validates OCM exchange codes for the /ocm/token endpoint.
// It resolves the accepted user and returns a shareId/resource-only scope
// suitable for minting exchanged JWTs (no long-lived shared secret in scope).
package ocmsharecode

import (
	"context"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocminvite "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	ocmshareutil "github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("ocmsharecode", New)
}

type manager struct {
	c *config
}

type config struct {
	GatewayAddr string `mapstructure:"gatewaysvc"`
}

func (c *config) ApplyDefaults() {
	c.GatewayAddr = sharedconf.GetGatewaySVC(c.GatewayAddr)
}

// New creates a new ocmsharecode authentication manager.
func New(ctx context.Context, m map[string]any) (auth.Manager, error) {
	var mgr manager
	if err := mgr.Configure(m); err != nil {
		return nil, err
	}
	return &mgr, nil
}

func (m *manager) Configure(ml map[string]any) error {
	var c config
	if err := cfg.Decode(ml, &c); err != nil {
		return errors.Wrap(err, "ocmsharecode: error decoding config")
	}
	m.c = &c
	return nil
}

// Authenticate validates an exchange code (the existing long-lived OCM WebDAV shared secret)
// and returns the accepted user with a shareId/resource-only scope for JWT minting.
// The generic auth interface passes OAuth client_id as the first argument. In OCM code flow
// that identifies the receiving server, so share lookup must come from the exchanged code.
// TODO(lopresti) here we could also enforce that the domain of the remote request matches
// the domain recorded in `shareRes`, taking into account forwarded host headers etc.
func (m *manager) Authenticate(ctx context.Context, clientID, code string) (*userpb.User, map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx).With().Str("client_id", clientID).Logger()

	gw, err := service.Gateway(ctx)
	if err != nil {
		return nil, nil, err
	}

	shareRes, err := gw.GetOCMShareByToken(ctx, &ocm.GetOCMShareByTokenRequest{
		Token: code,
	})

	switch {
	case err != nil:
		log.Error().Err(err).Msg("error getting ocm share by code")
		return nil, nil, err
	case shareRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(shareRes.Status.Message)
	case shareRes.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
		return nil, nil, errtypes.InvalidCredentials(shareRes.Status.Message)
	case shareRes.Status.Code != rpc.Code_CODE_OK:
		return nil, nil, errtypes.InternalError(shareRes.Status.Message)
	}

	// Resolve the accepted user (same pattern as ocmshares)
	u := shareRes.Share.Grantee.GetUserId()
	d, err := utils.MarshalProtoV1ToJSON(shareRes.GetShare().Creator)
	if err != nil {
		return nil, nil, err
	}

	o := &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"user-filter": {
				Decoder: "json",
				Value:   d,
			},
		},
	}

	userRes, err := gw.GetAcceptedUser(ctx, &ocminvite.GetAcceptedUserRequest{
		RemoteUserId: u,
		Opaque:       o,
	})

	switch {
	case err != nil:
		return nil, nil, err
	case userRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(userRes.Status.Message)
	case userRes.Status.Code != rpc.Code_CODE_OK:
		return nil, nil, errtypes.InternalError(userRes.Status.Message)
	}

	role, roleStr := ocmshareutil.GetRole(shareRes.Share)

	// Use code-flow scope: shareId/resource-only, no embedded shared secret
	s, err := scope.AddCodeFlowOCMShareScope(shareRes.Share, role, nil)
	if err != nil {
		return nil, nil, err
	}

	user := userRes.RemoteUser
	user.Opaque = &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"ocm-share-role": {
				Decoder: "plain",
				Value:   []byte(roleStr),
			},
		},
	}

	return user, s, nil
}
