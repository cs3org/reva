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

package gateway

import (
	"context"
	"fmt"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	registry "github.com/cs3org/go-cs3apis/cs3/auth/registry/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	storageprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

func (s *svc) Authenticate(ctx context.Context, req *gateway.AuthenticateRequest) (*gateway.AuthenticateResponse, error) {
	log := appctx.GetLogger(ctx)

	// find auth provider
	c, err := s.findAuthProvider(ctx, req.Type)
	if err != nil {
		err = errtypes.NotFound("gateway: error finding auth provider for type: " + req.Type)
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "error getting auth provider client"),
		}, nil
	}

	authProviderReq := &authpb.AuthenticateRequest{
		ClientId:     req.ClientId,
		ClientSecret: req.ClientSecret,
	}
	res, err := c.Authenticate(ctx, authProviderReq)
	switch {
	case err != nil:
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, fmt.Sprintf("gateway: error calling Authenticate for type: %s", req.Type)),
		}, nil
	case res.Status.Code == rpc.Code_CODE_PERMISSION_DENIED:
		fallthrough
	case res.Status.Code == rpc.Code_CODE_UNAUTHENTICATED:
		fallthrough
	case res.Status.Code == rpc.Code_CODE_NOT_FOUND:
		// normal failures, no need to log
		return &gateway.AuthenticateResponse{
			Status: res.Status,
		}, nil
	case res.Status.Code != rpc.Code_CODE_OK:
		err := status.NewErrorFromCode(res.Status.Code, "gateway")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, fmt.Sprintf("error authenticating credentials to auth provider for type: %s", req.Type)),
		}, nil
	}

	// validate valid userId
	if res.User == nil {
		err := errtypes.NotFound("gateway: user after Authenticate is nil")
		log.Err(err).Msg("user is nil")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "user is nil"),
		}, nil
	}

	if res.User.Id == nil {
		err := errtypes.NotFound("gateway: uid after Authenticate is nil")
		log.Err(err).Msg("user id is nil")
		return &gateway.AuthenticateResponse{
			Status: status.NewInternal(ctx, err, "user id is nil"),
		}, nil
	}

	u := *res.User
	if sharedconf.SkipUserGroupsInToken() {
		u.Groups = []string{}
	}

	// We need to expand the scopes of lightweight accounts, user shares and
	// public shares, for which we need to retrieve the receieved shares and stat
	// the resources referenced by these. Since the current scope can do that,
	// mint a temporary token based on that and expand the scope. Then set the
	// token obtained from the updated scope in the context.
	token, err := s.tokenmgr.MintToken(ctx, &u, res.TokenScope)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in MintToken")
		res := &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error creating access token"),
		}
		return res, nil
	}

	ctx = ctxpkg.ContextSetToken(ctx, token)
	ctx = ctxpkg.ContextSetUser(ctx, res.User)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, token)

	// Commenting out as the token size can get too big
	// For now, we'll try to resolve all resources on every request and cache those
	/* scope, err := s.expandScopes(ctx, res.TokenScope)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error expanding token scope")
		return &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error expanding access token scope"),
		}, nil
	}
	*/
	scope := res.TokenScope

	token, err = s.tokenmgr.MintToken(ctx, &u, scope)
	if err != nil {
		err = errors.Wrap(err, "authsvc: error in MintToken")
		res := &gateway.AuthenticateResponse{
			Status: status.NewUnauthenticated(ctx, err, "error creating access token"),
		}
		return res, nil
	}

	if scope, ok := res.TokenScope["user"]; s.c.DisableHomeCreationOnLogin || !ok || scope.Role != authpb.Role_ROLE_OWNER || res.User.Id.Type == userpb.UserType_USER_TYPE_FEDERATED {
		gwRes := &gateway.AuthenticateResponse{
			Status: status.NewOK(ctx),
			User:   res.User,
			Token:  token,
		}
		return gwRes, nil
	}

	// we need to pass the token to authenticate the CreateHome request.
	// TODO(labkode): appending to existing context will not pass the token.
	ctx = ctxpkg.ContextSetToken(ctx, token)
	ctx = ctxpkg.ContextSetUser(ctx, res.User)
	ctx = metadata.AppendToOutgoingContext(ctx, ctxpkg.TokenHeader, token) // TODO(jfd): hardcoded metadata key. use  PerRPCCredentials?

	// create home directory
	if _, err = s.createHomeCache.Get(res.User.Id.OpaqueId); err != nil {
		createHomeRes, err := s.CreateHome(ctx, &storageprovider.CreateHomeRequest{})
		if err != nil {
			log.Err(err).Msg("error calling CreateHome")
			return &gateway.AuthenticateResponse{
				Status: status.NewInternal(ctx, err, "error creating user home"),
			}, nil
		}

		if createHomeRes.Status.Code != rpc.Code_CODE_OK {
			err := status.NewErrorFromCode(createHomeRes.Status.Code, "gateway")
			log.Err(err).Msg("error calling Createhome")
			return &gateway.AuthenticateResponse{
				Status: status.NewInternal(ctx, err, "error creating user home"),
			}, nil
		}
		if s.c.CreateHomeCacheTTL > 0 {
			_ = s.createHomeCache.Set(res.User.Id.OpaqueId, true)
		}
	}

	gwRes := &gateway.AuthenticateResponse{
		Status: status.NewOK(ctx),
		User:   res.User,
		Token:  token,
	}
	return gwRes, nil
}

func (s *svc) WhoAmI(ctx context.Context, req *gateway.WhoAmIRequest) (*gateway.WhoAmIResponse, error) {
	u, _, err := s.tokenmgr.DismantleToken(ctx, req.Token)
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting user from token")
		return &gateway.WhoAmIResponse{
			Status: status.NewUnauthenticated(ctx, err, "error dismantling token"),
		}, nil
	}

	if sharedconf.SkipUserGroupsInToken() {
		groupsRes, err := s.GetUserGroups(ctx, &userpb.GetUserGroupsRequest{UserId: u.Id})
		if err != nil {
			return nil, err
		}
		u.Groups = groupsRes.Groups
	}

	res := &gateway.WhoAmIResponse{
		Status: status.NewOK(ctx),
		User:   u,
	}
	return res, nil
}

func (s *svc) findAuthProvider(ctx context.Context, authType string) (authpb.ProviderAPIClient, error) {
	c, err := pool.GetAuthRegistryServiceClient(pool.Endpoint(s.c.AuthRegistryEndpoint))
	if err != nil {
		err = errors.Wrap(err, "gateway: error getting auth registry client")
		return nil, err
	}

	res, err := c.GetAuthProviders(ctx, &registry.GetAuthProvidersRequest{
		Type: authType,
	})

	if err != nil {
		err = errors.Wrap(err, "gateway: error calling GetAuthProvider")
		return nil, err
	}

	if res.Status.Code == rpc.Code_CODE_OK && res.Providers != nil && len(res.Providers) > 0 {
		// TODO(labkode): check for capabilities here
		c, err := pool.GetAuthProviderServiceClient(pool.Endpoint(res.Providers[0].Address))
		if err != nil {
			err = errors.Wrap(err, "gateway: error getting an auth provider client")
			return nil, err
		}

		return c, nil
	}

	if res.Status.Code == rpc.Code_CODE_NOT_FOUND {
		return nil, errtypes.NotFound("gateway: auth provider not found for type:" + authType)
	}

	return nil, errtypes.InternalError("gateway: error finding an auth provider for type: " + authType)
}

/*
func (s *svc) expandScopes(ctx context.Context, scopeMap map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	log := appctx.GetLogger(ctx)
	newMap := make(map[string]*authpb.Scope)

	for k, v := range scopeMap {
		newMap[k] = v
		switch {
		case strings.HasPrefix(k, "publicshare"):
			var share link.PublicShare
			err := utils.UnmarshalJSONToProtoV1(v.Resource.Value, &share)
			if err != nil {
				log.Warn().Err(err).Msgf("error unmarshalling public share %+v", v.Resource.Value)
				continue
			}
			newMap, err = s.statAndAddResource(ctx, share.ResourceId, v.Role, newMap)
			if err != nil {
				log.Warn().Err(err).Msgf("error expanding publicshare resource scope %+v", share.ResourceId)
				continue
			}

		case strings.HasPrefix(k, "share"):
			var share collaboration.Share
			err := utils.UnmarshalJSONToProtoV1(v.Resource.Value, &share)
			if err != nil {
				log.Warn().Err(err).Msgf("error unmarshalling share %+v", v.Resource.Value)
				continue
			}
			newMap, err = s.statAndAddResource(ctx, share.ResourceId, v.Role, newMap)
			if err != nil {
				log.Warn().Err(err).Msgf("error expanding share resource scope %+v", share.ResourceId)
				continue
			}

		case strings.HasPrefix(k, "lightweight"):
			shares, err := s.ListReceivedShares(ctx, &collaboration.ListReceivedSharesRequest{})
			if err != nil || shares.Status.Code != rpc.Code_CODE_OK {
				log.Warn().Err(err).Msg("error listing received shares")
				continue
			}
			for _, share := range shares.Shares {
				newMap, err = scope.AddReceivedShareScope(share, v.Role, newMap)
				if err != nil {
					log.Warn().Err(err).Msgf("error expanding received share scope %+v", share.Share.ResourceId)
					continue
				}
				newMap, err = s.statAndAddResource(ctx, share.Share.ResourceId, v.Role, newMap)
				if err != nil {
					log.Warn().Err(err).Msgf("error expanding received share resource scope %+v", share.Share.ResourceId)
					continue
				}
			}
		}
	}
	return newMap, nil
}

func (s *svc) statAndAddResource(ctx context.Context, r *storageprovider.ResourceId, role authpb.Role, scopeMap map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
	statReq := &storageprovider.StatRequest{
		Ref: &storageprovider.Reference{ResourceId: r},
	}
	statResponse, err := s.Stat(ctx, statReq)
	if err != nil {
		return scopeMap, err
	}
	if statResponse.Status.Code != rpc.Code_CODE_OK {
		return scopeMap, status.NewErrorFromCode(statResponse.Status.Code, "authprovider")
	}

	return scope.AddResourceInfoScope(statResponse.Info, role, scopeMap)
}
*/
