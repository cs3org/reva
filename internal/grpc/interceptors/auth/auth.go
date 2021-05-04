// Copyright 2018-2021 CERN
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
	"fmt"
	"strings"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/auth/scope"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/token"
	tokenmgr "github.com/cs3org/reva/pkg/token/manager/registry"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type config struct {
	// TODO(labkode): access a map is more performant as uri as fixed in length
	// for SkipMethods.
	TokenManager  string                            `mapstructure:"token_manager"`
	TokenManagers map[string]map[string]interface{} `mapstructure:"token_managers"`
	GatewayAddr   string                            `mapstructure:"gateway_addr"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "auth: error decoding conf")
		return nil, err
	}
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

	if conf.TokenManager == "" {
		conf.TokenManager = "jwt"
	}
	conf.GatewayAddr = sharedconf.GetGatewaySVC(conf.GatewayAddr)

	h, ok := tokenmgr.NewFuncs[conf.TokenManager]
	if !ok {
		return nil, errtypes.NotFound("auth: token manager does not exist: " + conf.TokenManager)
	}

	tokenManager, err := h(conf.TokenManagers[conf.TokenManager])
	if err != nil {
		return nil, errors.Wrap(err, "auth: error creating token manager")
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx, span := trace.StartSpan(ctx, "auth")
		defer span.End()
		log := appctx.GetLogger(ctx)

		if utils.Skip(info.FullMethod, unprotected) {
			span.AddAttributes(trace.BoolAttribute("auth_enabled", false))
			log.Debug().Str("method", info.FullMethod).Msg("skipping auth")

			// If a token is present, set it anyway, as we might need the user info
			// to decide the storage provider.
			tkn, ok := token.ContextGetToken(ctx)
			if ok {
				u, err := dismantleToken(ctx, tkn, req, tokenManager, conf.GatewayAddr)
				if err == nil {
					ctx = user.ContextSetUser(ctx, u)
				}
			}
			return handler(ctx, req)
		}

		span.AddAttributes(trace.BoolAttribute("auth_enabled", true))

		tkn, ok := token.ContextGetToken(ctx)

		if !ok || tkn == "" {
			log.Warn().Msg("access token not found or empty")
			return nil, status.Errorf(codes.Unauthenticated, "auth: core access token not found")
		}

		// validate the token and ensure access to the resource is allowed
		u, err := dismantleToken(ctx, tkn, req, tokenManager, conf.GatewayAddr)
		if err != nil {
			log.Warn().Err(err).Msg("access token is invalid")
			return nil, status.Errorf(codes.Unauthenticated, "auth: core access token is invalid")
		}

		// store user and core access token in context.
		span.AddAttributes(
			trace.StringAttribute("id.idp", u.Id.Idp),
			trace.StringAttribute("id.opaque_id", u.Id.OpaqueId),
			trace.StringAttribute("username", u.Username),
			trace.StringAttribute("token", tkn))
		span.AddAttributes(trace.StringAttribute("user", u.String()), trace.StringAttribute("token", tkn))

		ctx = user.ContextSetUser(ctx, u)
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
			tkn, ok := token.ContextGetToken(ctx)
			if ok {
				u, err := dismantleToken(ctx, tkn, ss, tokenManager, conf.GatewayAddr)
				if err == nil {
					ctx = user.ContextSetUser(ctx, u)
					ss = newWrappedServerStream(ctx, ss)
				}
			}

			return handler(srv, ss)
		}

		tkn, ok := token.ContextGetToken(ctx)

		if !ok || tkn == "" {
			log.Warn().Msg("access token not found")
			return status.Errorf(codes.Unauthenticated, "auth: core access token not found")
		}

		// validate the token and ensure access to the resource is allowed
		u, err := dismantleToken(ctx, tkn, ss, tokenManager, conf.GatewayAddr)
		if err != nil {
			log.Warn().Err(err).Msg("access token is invalid")
			return status.Errorf(codes.Unauthenticated, "auth: core access token is invalid")
		}

		// store user and core access token in context.
		ctx = user.ContextSetUser(ctx, u)
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

func dismantleToken(ctx context.Context, tkn string, req interface{}, mgr token.Manager, gatewayAddr string) (*userpb.User, error) {
	log := appctx.GetLogger(ctx)
	u, tokenScope, err := mgr.DismantleToken(ctx, tkn)
	if err != nil {
		return nil, err
	}

	// Check if access to the resource is in the scope of the token
	ok, err := scope.VerifyScope(tokenScope, req)
	if err != nil {
		return nil, errtypes.InternalError("error verifying scope of access token")
	}
	if ok {
		return u, nil
	}

	// Check if req is of type *provider.Reference_Path
	// If yes, the request might be coming from a share where the accessor is
	// trying to impersonate the owner, since the share manager doesn't know the
	// share path.
	if ref, ok := extractRef(req); ok {
		if ref.GetPath() != "" {

			// Try to extract the resource ID from the scope resource.
			// Currently, we only check for public shares, but this will be extended
			// for OCM shares, guest accounts, etc.
			log.Info().Msgf("resolving path reference to ID to check token scope %+v", ref.GetPath())
			var share link.PublicShare
			err = utils.UnmarshalJSONToProtoV1(tokenScope["publicshare"].Resource.Value, &share)
			if err != nil {
				return nil, err
			}

			client, err := pool.GetGatewayServiceClient(gatewayAddr)
			if err != nil {
				return nil, err
			}

			// Since the public share is obtained from the scope, the current token
			// has access to it.
			statReq := &provider.StatRequest{
				Ref: &provider.Reference{
					Spec: &provider.Reference_Id{Id: share.ResourceId},
				},
			}

			statResponse, err := client.Stat(ctx, statReq)
			if err != nil || statResponse.Status.Code != rpc.Code_CODE_OK {
				return nil, err
			}

			if strings.HasPrefix(ref.GetPath(), statResponse.Info.Path) {
				// The path corresponds to the resource to which the token has access.
				// We allow access to it.
				return u, nil
			}
		}
	}

	return nil, err
}

func extractRef(req interface{}) (*provider.Reference, bool) {
	switch v := req.(type) {
	case *registry.GetStorageProvidersRequest:
		return v.GetRef(), true
	case *provider.StatRequest:
		return v.GetRef(), true
	case *provider.ListContainerRequest:
		return v.GetRef(), true
	case *provider.CreateContainerRequest:
		return v.GetRef(), true
	case *provider.DeleteRequest:
		return v.GetRef(), true
	case *provider.MoveRequest:
		return v.GetSource(), true
	case *provider.InitiateFileDownloadRequest:
		return v.GetRef(), true
	case *provider.InitiateFileUploadRequest:
		return v.GetRef(), true
	}
	return nil, false
}
