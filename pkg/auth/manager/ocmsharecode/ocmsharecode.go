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
	"strings"

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
	"github.com/cs3org/reva/v3/pkg/ocm/providerdomain"
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

// Authenticate validates an exchange code and the receiving provider named by
// clientID. clientID must be the host-only FQDN of the share's stored
// recipient. The exchanged code is the only share lookup key.
func (m *manager) Authenticate(
	ctx context.Context,
	clientID string,
	code string,
) (*userpb.User, map[string]*authpb.Scope, error) {
	if err := providerdomain.Validate(clientID); err != nil {
		return nil, nil, errtypes.InvalidCredentials("invalid ocm client_id")
	}

	log := appctx.GetLogger(ctx).With().Str("client_id", clientID).Logger()

	gw, err := service.Gateway(ctx)
	if err != nil {
		return nil, nil, err
	}

	shareRes, err := gw.GetOCMShareByToken(ctx, &ocm.GetOCMShareByTokenRequest{
		Token: code,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting ocm share by code")
		return nil, nil, err
	}
	if shareRes == nil || shareRes.Status == nil {
		return nil, nil, errtypes.InternalError("missing ocm share response")
	}

	switch shareRes.Status.Code {
	case rpc.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(shareRes.Status.Message)
	case rpc.Code_CODE_PERMISSION_DENIED:
		return nil, nil, errtypes.InvalidCredentials(shareRes.Status.Message)
	case rpc.Code_CODE_OK:
	default:
		return nil, nil, errtypes.InternalError(shareRes.Status.Message)
	}

	share := shareRes.GetShare()
	// providerId is the resolved share's opaque id. A missing share, a missing
	// id, or a blank opaque id cannot authenticate a code-flow token.
	if strings.TrimSpace(share.GetId().GetOpaqueId()) == "" {
		return nil, nil, errtypes.InvalidCredentials("ocm share is missing provider id")
	}

	// The stored recipient is the grantee IdP. It must be a host-only FQDN and
	// match clientID without regard to case. Sender and creator are not compared.
	grantee := share.GetGrantee().GetUserId()
	if grantee == nil {
		return nil, nil, errtypes.InvalidCredentials("ocm share is missing grantee")
	}
	recipient := grantee.GetIdp()
	recipientInvalid := providerdomain.Validate(recipient) != nil
	recipientMismatch := !strings.EqualFold(clientID, recipient)
	if recipientInvalid || recipientMismatch {
		return nil, nil, errtypes.InvalidCredentials("ocm share receiver does not match client_id")
	}

	// Resolve the accepted user (same pattern as ocmshares)
	u := grantee
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
	case userRes == nil || userRes.Status == nil:
		return nil, nil, errtypes.InternalError("missing accepted user response")
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
