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

package auth

import (
	"context"
	"time"

	"github.com/bluele/gcache"
	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gatewayv1beta1 "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/scope"
	ctxpkg "github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	tokenmgr "github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var userGroupsCache gcache.Cache
var scopeExpansionCache gcache.Cache

type config struct {
	// TODO(labkode): access a map is more performant as uri as fixed in length
	// for SkipMethods.
	TokenManager  string                            `mapstructure:"token_manager"`
	TokenManagers map[string]map[string]interface{} `mapstructure:"token_managers"`
	GatewayAddr   string                            `mapstructure:"gateway_addr"`
	blockedUsers  []string
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "auth: error decoding conf")
		return nil, err
	}
	c.blockedUsers = sharedconf.GetBlockedUsers()
	return c, nil
}

// NewUnary returns a new unary interceptor that adds
// trace information for the request.
func NewUnary(m map[string]interface{}, unprotected []string) (grpc.UnaryServerInterceptor, error) {
	conf, err := parseConfig(m)
	if err != nil {
		err = errors.Wrap(err, "auth: error parsing config")
		return nil, err
	}

	blockedUsers := user.NewBlockedUsersSet(conf.blockedUsers)

	if conf.TokenManager == "" {
		conf.TokenManager = "jwt"
	}
	conf.GatewayAddr = sharedconf.GetGatewaySVC(conf.GatewayAddr)

	userGroupsCache = gcache.New(1000000).LFU().Build()
	scopeExpansionCache = gcache.New(1000000).LFU().Build()

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, errtypes.NotFound("auth: token manager does not exist: " + conf.TokenManager)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, errors.Wrap(err, "auth: error creating token manager")
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		log := appctx.GetLogger(ctx)

		if utils.Skip(info.FullMethod, unprotected) {
			log.Debug().Str("method", info.FullMethod).Msg("skipping auth")

			// If a token is present, set it anyway, as we might need the user info
			// to decide the storage provider.
			tkn, ok := ctxpkg.ContextGetToken(ctx)
			if ok {
				u, scopes, err := dismantleToken(ctx, tkn, req, tokenManager, conf.GatewayAddr, true)
				if err == nil {
					if blockedUsers.IsBlocked(u.Username) {
						return nil, status.Errorf(codes.PermissionDenied, "user %s blocked", u.Username)
					}
					ctx = ctxpkg.ContextSetUser(ctx, u)
					ctx = ctxpkg.ContextSetScopes(ctx, scopes)
				}
			}
			return handler(ctx, req)
		}

		tkn, ok := ctxpkg.ContextGetToken(ctx)

		if !ok || tkn == "" {
			log.Warn().Msg("access token not found or empty")
			return nil, status.Errorf(codes.Unauthenticated, "auth: core access token not found")
		}

		// scopes, validate the token and ensure access to the resource is allowed
		u, scopes, err := dismantleToken(ctx, tkn, req, tokenManager, conf.GatewayAddr, false)
		if err != nil {
			log.Warn().Err(err).Msg("access token is invalid")
			return nil, status.Errorf(codes.PermissionDenied, "auth: core access token is invalid")
		}

		if blockedUsers.IsBlocked(u.Username) {
			return nil, status.Errorf(codes.PermissionDenied, "user %s blocked", u.Username)
		}

		ctx = ctxpkg.ContextSetUser(ctx, u)
		ctx = ctxpkg.ContextSetScopes(ctx, scopes)
		return handler(ctx, req)
	}
	return interceptor, nil
}

// NewStream returns a new server stream interceptor
// that adds trace information to the request.
func NewStream(m map[string]interface{}, unprotected []string) (grpc.StreamServerInterceptor, error) {
	conf, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	if conf.TokenManager == "" {
		conf.TokenManager = "jwt"
	}

	userGroupsCache = gcache.New(1000000).LFU().Build()
	scopeExpansionCache = gcache.New(1000000).LFU().Build()

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, errtypes.NotFound("auth: token manager not found: " + conf.TokenManager)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, errtypes.NotFound("auth: token manager not found: " + conf.TokenManager)
	}

	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		log := appctx.GetLogger(ctx)

		if utils.Skip(info.FullMethod, unprotected) {
			log.Debug().Str("method", info.FullMethod).Msg("skipping auth")

			// If a token is present, set it anyway, as we might need the user info
			// to decide the storage provider.
			tkn, ok := ctxpkg.ContextGetToken(ctx)
			if ok {
				u, scopes, err := dismantleToken(ctx, tkn, ss, tokenManager, conf.GatewayAddr, true)
				if err == nil {
					ctx = ctxpkg.ContextSetUser(ctx, u)
					ctx = ctxpkg.ContextSetScopes(ctx, scopes)
					ss = newWrappedServerStream(ctx, ss)
				}
			}

			return handler(srv, ss)
		}

		tkn, ok := ctxpkg.ContextGetToken(ctx)

		if !ok || tkn == "" {
			log.Warn().Msg("access token not found")
			return status.Errorf(codes.Unauthenticated, "auth: core access token not found")
		}

		// validate the token and ensure access to the resource is allowed
		u, scopes, err := dismantleToken(ctx, tkn, ss, tokenManager, conf.GatewayAddr, false)
		if err != nil {
			log.Warn().Err(err).Msg("access token is invalid")
			return status.Errorf(codes.PermissionDenied, "auth: core access token is invalid")
		}

		// store user and core access token in context.
		ctx = ctxpkg.ContextSetUser(ctx, u)
		ctx = ctxpkg.ContextSetScopes(ctx, scopes)
		wrapped := newWrappedServerStream(ctx, ss)
		return handler(srv, wrapped)
	}
	return interceptor, nil
}

func newWrappedServerStream(ctx context.Context, ss grpc.ServerStream) *wrappedServerStream {
	return &wrappedServerStream{ServerStream: ss, newCtx: ctx}
}

type wrappedServerStream struct {
	grpc.ServerStream
	newCtx context.Context
}

func (ss *wrappedServerStream) Context() context.Context {
	return ss.newCtx
}

func dismantleToken(ctx context.Context, tkn string, req interface{}, mgr token.Manager, gatewayAddr string, unprotected bool) (*userpb.User, map[string]*authpb.Scope, error) {
	u, tokenScope, err := mgr.DismantleToken(ctx, tkn)
	if err != nil {
		return nil, nil, err
	}

	if unprotected {
		return u, nil, nil
	}

	if sharedconf.SkipUserGroupsInToken() {
		client, err := pool.GetGatewayServiceClient(pool.Endpoint(gatewayAddr))
		if err != nil {
			return nil, nil, err
		}
		groups, err := getUserGroups(ctx, u, client)
		if err != nil {
			return nil, nil, err
		}
		u.Groups = groups
	}

	// Check if access to the resource is in the scope of the token
	ok, err := scope.VerifyScope(ctx, tokenScope, req)
	if err != nil {
		return nil, nil, errtypes.InternalError("error verifying scope of access token")
	}
	if ok {
		return u, tokenScope, nil
	}

	if err = expandAndVerifyScope(ctx, req, tokenScope, u, gatewayAddr, mgr); err != nil {
		return nil, nil, err
	}

	return u, tokenScope, nil
}

func getUserGroups(ctx context.Context, u *userpb.User, client gatewayv1beta1.GatewayAPIClient) ([]string, error) {
	if groupsIf, err := userGroupsCache.Get(u.Id.OpaqueId); err == nil {
		log := appctx.GetLogger(ctx)
		log.Info().Msgf("user groups found in cache %s", u.Id.OpaqueId)
		return groupsIf.([]string), nil
	}

	res, err := client.GetUserGroups(ctx, &userpb.GetUserGroupsRequest{UserId: u.Id})
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling GetUserGroups")
	}
	_ = userGroupsCache.SetWithExpire(u.Id.OpaqueId, res.Groups, 3600*time.Second)

	return res.Groups, nil
}
