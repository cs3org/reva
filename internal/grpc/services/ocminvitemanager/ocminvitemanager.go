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

package ocminvitemanager

import (
	"context"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocminvitemanager", New)
	plugin.RegisterNamespace("grpc.services.ocminvitemanager.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver            string                            `mapstructure:"driver"`
	Drivers           map[string]map[string]interface{} `mapstructure:"drivers"`
	TokenExpiration   string                            `mapstructure:"token_expiration"`
	OCMClientTimeout  int                               `mapstructure:"ocm_timeout"`
	OCMClientInsecure bool                              `mapstructure:"ocm_insecure"`
	GatewaySVC        string                            `mapstructure:"gatewaysvc"                                    validate:"required"`
	ProviderDomain    string                            `docs:"The same domain registered in the provider authorizer" mapstructure:"provider_domain" validate:"required"`

	tokenExpiration time.Duration
}

type service struct {
	conf      *config
	repo      invite.Repository
	ocmClient *ocmd.OCMClient
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
	if c.TokenExpiration == "" {
		c.TokenExpiration = "24h"
	}

	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

func (s *service) Register(ss *grpc.Server) {
	invitepb.RegisterInviteAPIServer(ss, s)
}

func getInviteRepository(ctx context.Context, c *config) (invite.Repository, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// New creates a new OCM invite manager svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	p, err := time.ParseDuration(c.TokenExpiration)
	if err != nil {
		return nil, err
	}
	c.tokenExpiration = p

	repo, err := getInviteRepository(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:      &c,
		repo:      repo,
		ocmClient: ocmd.NewClient(time.Duration(c.OCMClientTimeout)*time.Second, c.OCMClientInsecure),
	}
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.ocm.invite.v1beta1.InviteAPI/AcceptInvite", "/cs3.ocm.invite.v1beta1.InviteAPI/GetAcceptedUser"}
}

func (s *service) GenerateInviteToken(ctx context.Context, req *invitepb.GenerateInviteTokenRequest) (*invitepb.GenerateInviteTokenResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	token := CreateToken(s.conf.tokenExpiration, user.GetId(), req.Description)

	if err := s.repo.AddToken(ctx, token); err != nil {
		return &invitepb.GenerateInviteTokenResponse{
			Status: status.NewInternal(ctx, err, "error generating invite token"),
		}, nil
	}

	return &invitepb.GenerateInviteTokenResponse{
		Status:      status.NewOK(ctx),
		InviteToken: token,
	}, nil
}

func (s *service) ListInviteTokens(ctx context.Context, req *invitepb.ListInviteTokensRequest) (*invitepb.ListInviteTokensResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	tokens, err := s.repo.ListTokens(ctx, user.Id)
	if err != nil {
		return &invitepb.ListInviteTokensResponse{
			Status: status.NewInternal(ctx, err, "error listing tokens"),
		}, nil
	}
	return &invitepb.ListInviteTokensResponse{
		Status:       status.NewOK(ctx),
		InviteTokens: tokens,
	}, nil
}

func (s *service) ForwardInvite(ctx context.Context, req *invitepb.ForwardInviteRequest) (*invitepb.ForwardInviteResponse, error) {
	user := appctx.ContextMustGetUser(ctx)

	ocmEndpoint, err := getOCMEndpoint(req.GetOriginSystemProvider())
	if err != nil {
		return nil, err
	}

	remoteUser, err := s.ocmClient.InviteAccepted(ctx, ocmEndpoint, &ocmd.InviteAcceptedRequest{
		Token:             req.InviteToken.GetToken(),
		RecipientProvider: s.conf.ProviderDomain,
		UserID:            user.GetId().GetOpaqueId(),
		Email:             user.GetMail(),
		Name:              user.GetDisplayName(),
	})
	if err != nil {
		switch {
		case errors.Is(err, ocmd.ErrTokenInvalid):
			return &invitepb.ForwardInviteResponse{
				Status: status.NewInvalid(ctx, "token invalid or not found"),
			}, nil
		case errors.Is(err, ocmd.ErrUserAlreadyAccepted):
			return &invitepb.ForwardInviteResponse{
				Status: status.NewAlreadyExists(ctx, err, err.Error()),
			}, nil
		case errors.Is(err, ocmd.ErrServiceNotTrusted):
			return &invitepb.ForwardInviteResponse{
				Status: status.NewPermissionDenied(ctx, err, err.Error()),
			}, nil
		default:
			return &invitepb.ForwardInviteResponse{
				Status: status.NewInternal(ctx, err, err.Error()),
			}, nil
		}
	}

	// create a link between the user that accepted the share (in ctx)
	// and the remote one (the initiator), so at the end of the invitation workflow they
	// know each other

	remoteUserID := &userpb.UserId{
		Type:     userpb.UserType_USER_TYPE_FEDERATED,
		Idp:      req.GetOriginSystemProvider().Domain,
		OpaqueId: remoteUser.UserID,
	}

	if err := s.repo.AddRemoteUser(ctx, user.Id, &userpb.User{
		Id:          remoteUserID,
		Mail:        remoteUser.Email,
		DisplayName: remoteUser.Name,
	}); err != nil {
		if !errors.Is(err, invite.ErrUserAlreadyAccepted) {
			// skip error if user was already accepted
			return &invitepb.ForwardInviteResponse{
				Status: status.NewInternal(ctx, err, err.Error()),
			}, nil
		}
	}

	return &invitepb.ForwardInviteResponse{
		Status:      status.NewOK(ctx),
		UserId:      remoteUserID,
		Email:       remoteUser.Email,
		DisplayName: remoteUser.Name,
	}, nil
}

func getOCMEndpoint(originProvider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range originProvider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Endpoint.Path, nil
		}
	}
	return "", errors.New("ocm endpoint not specified for mesh provider")
}

func (s *service) AcceptInvite(ctx context.Context, req *invitepb.AcceptInviteRequest) (*invitepb.AcceptInviteResponse, error) {
	token, err := s.repo.GetToken(ctx, req.InviteToken.Token)
	if err != nil {
		if errors.Is(err, invite.ErrTokenNotFound) {
			return &invitepb.AcceptInviteResponse{
				Status: status.NewInvalid(ctx, "token invalid or not found"),
			}, nil
		}
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	if !isTokenValid(token) {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInvalid(ctx, "token invalid or not found"),
		}, nil
	}

	initiator, err := s.getUserInfo(ctx, token.UserId)
	if err != nil {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	if err := s.repo.AddRemoteUser(ctx, token.GetUserId(), req.GetRemoteUser()); err != nil {
		if errors.Is(err, invite.ErrUserAlreadyAccepted) {
			return &invitepb.AcceptInviteResponse{
				Status: status.NewAlreadyExists(ctx, err, err.Error()),
			}, nil
		}
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	return &invitepb.AcceptInviteResponse{
		Status:      status.NewOK(ctx),
		UserId:      initiator.GetId(),
		Email:       initiator.Mail,
		DisplayName: initiator.DisplayName,
	}, nil
}

func (s *service) getUserInfo(ctx context.Context, id *userpb.UserId) (*userpb.User, error) {
	gw, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySVC))
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUser(ctx, &userpb.GetUserRequest{
		UserId: id,
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}

	return res.User, nil
}

func isTokenValid(token *invitepb.InviteToken) bool {
	return time.Now().Unix() < int64(token.Expiration.Seconds)
}

func (s *service) GetAcceptedUser(ctx context.Context, req *invitepb.GetAcceptedUserRequest) (*invitepb.GetAcceptedUserResponse, error) {
	logger := appctx.GetLogger(ctx)
	user, ok := getUserFilter(ctx, req)
	if !ok {
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "user not found"),
		}, nil
	}

	logger.Info().Msgf("GetAcceptedUser %s at %s", user.Id.OpaqueId, user.Id.Idp)
	remoteUser, err := s.repo.GetRemoteUser(ctx, user.GetId(), req.GetRemoteUserId())
	if err != nil {
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewInternal(ctx, err, "error fetching remote user details"),
		}, nil
	}

	return &invitepb.GetAcceptedUserResponse{
		Status:     status.NewOK(ctx),
		RemoteUser: remoteUser,
	}, nil
}

func getUserFilter(ctx context.Context, req *invitepb.GetAcceptedUserRequest) (*userpb.User, bool) {
	user, ok := appctx.ContextGetUser(ctx)
	if ok {
		return user, true
	}

	if req.Opaque == nil || req.Opaque.Map == nil {
		return nil, false
	}

	v, ok := req.Opaque.Map["user-filter"]
	if !ok {
		return nil, false
	}

	var u userpb.UserId
	if err := utils.UnmarshalJSONToProtoV1(v.Value, &u); err != nil {
		return nil, false
	}
	return &userpb.User{Id: &u}, true
}

func (s *service) FindAcceptedUsers(ctx context.Context, req *invitepb.FindAcceptedUsersRequest) (*invitepb.FindAcceptedUsersResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	acceptedUsers, err := s.repo.FindRemoteUsers(ctx, user.GetId(), req.GetFilter())
	if err != nil {
		return &invitepb.FindAcceptedUsersResponse{
			Status: status.NewInternal(ctx, err, "error finding remote users: "+err.Error()),
		}, nil
	}

	return &invitepb.FindAcceptedUsersResponse{
		Status:        status.NewOK(ctx),
		AcceptedUsers: acceptedUsers,
	}, nil
}

func (s *service) DeleteAcceptedUser(ctx context.Context, req *invitepb.DeleteAcceptedUserRequest) (*invitepb.DeleteAcceptedUserResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	if err := s.repo.DeleteRemoteUser(ctx, user.Id, req.RemoteUserId); err != nil {
		return &invitepb.DeleteAcceptedUserResponse{
			Status: status.NewInternal(ctx, err, "error deleting remote users: "+err.Error()),
		}, nil
	}

	return &invitepb.DeleteAcceptedUserResponse{
		Status: status.NewOK(ctx),
	}, nil
}
