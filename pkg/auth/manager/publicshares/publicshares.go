// Copyright 2018-2024 CERN
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

package publicshares

import (
	"context"
	"strings"
	"time"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/auth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("publicshares", New)
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

// New returns a new auth Manager.
func New(ctx context.Context, m map[string]interface{}) (auth.Manager, error) {
	mgr := &manager{}
	err := mgr.Configure(m)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *manager) Configure(ml map[string]interface{}) error {
	var c config
	if err := cfg.Decode(ml, &c); err != nil {
		return errors.Wrap(err, "publicshares: error decoding config")
	}
	m.c = &c
	return nil
}

func (m *manager) Authenticate(ctx context.Context, token, secret string) (*user.User, map[string]*authpb.Scope, error) {
	gwConn, err := pool.GetGatewayServiceClient(pool.Endpoint(m.c.GatewayAddr))
	if err != nil {
		return nil, nil, err
	}

	log := appctx.GetLogger(ctx)

	var auth *link.PublicShareAuthentication
	if strings.HasPrefix(secret, "password|") {
		secret = strings.TrimPrefix(secret, "password|")
		auth = &link.PublicShareAuthentication{
			Spec: &link.PublicShareAuthentication_Password{
				Password: secret,
			},
		}
	} else if strings.HasPrefix(secret, "signature|") {
		secret = strings.TrimPrefix(secret, "signature|")
		parts := strings.Split(secret, "|")
		sig, expiration := parts[0], parts[1]
		exp, _ := time.Parse(time.RFC3339, expiration)

		auth = &link.PublicShareAuthentication{
			Spec: &link.PublicShareAuthentication_Signature{
				Signature: &link.ShareSignature{
					Signature: sig,
					SignatureExpiration: &types.Timestamp{
						Seconds: uint64(exp.UnixNano() / 1000000000),
						Nanos:   uint32(exp.UnixNano() % 1000000000),
					},
				},
			},
		}
	}

	log.Debug().Str("token", token).Msg("Handling Authenticate() call")

	publicShareResponse, err := gwConn.GetPublicShareByToken(ctx, &link.GetPublicShareByTokenRequest{
		Token:          token,
		Authentication: auth,
		Sign:           true,
	})
	log.Debug().Str("token", token).Err(err).Any("psresp", publicShareResponse).Msg("GetPublicShareByToken return")

	switch {
	case err != nil:
		return nil, nil, err
	case publicShareResponse.Status.Code == rpcv1beta1.Code_CODE_NOT_FOUND:
		return nil, nil, errtypes.NotFound(publicShareResponse.Status.Message)
	case publicShareResponse.Status.Code == rpcv1beta1.Code_CODE_PERMISSION_DENIED:
		return nil, nil, errtypes.InvalidCredentials(publicShareResponse.Status.Message)
	case publicShareResponse.Status.Code != rpcv1beta1.Code_CODE_OK:
		return nil, nil, errtypes.InternalError(publicShareResponse.Status.Message)
	}

	getUserResponse, err := gwConn.GetUser(ctx, &user.GetUserRequest{
		UserId: publicShareResponse.GetShare().GetOwner(),
	})
	if err != nil {
		return nil, nil, err
	}
	if getUserResponse.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, nil, errtypes.NotFound(getUserResponse.Status.Message)
	}

	share := publicShareResponse.GetShare()
	role := authpb.Role_ROLE_VIEWER
	roleStr := "viewer"
	if share.Permissions.Permissions.InitiateFileUpload && !share.Permissions.Permissions.InitiateFileDownload {
		role = authpb.Role_ROLE_UPLOADER
		roleStr = "uploader"
	} else if share.Permissions.Permissions.InitiateFileUpload {
		role = authpb.Role_ROLE_EDITOR
		roleStr = "editor"
	}

	scope, err := scope.AddPublicShareScope(share, role, nil)
	if err != nil {
		return nil, nil, err
	}

	u := getUserResponse.GetUser()
	u.Opaque = &types.Opaque{
		Map: map[string]*types.OpaqueEntry{
			"public-share-role": {
				Decoder: "plain",
				Value:   []byte(roleStr),
			},
		},
	}

	return u, scope, nil
}

// ErrPasswordNotProvided is returned when the public share is password protected, but there was no password on the request.
var ErrPasswordNotProvided = errors.New("public share is password protected, but password was not provided")
