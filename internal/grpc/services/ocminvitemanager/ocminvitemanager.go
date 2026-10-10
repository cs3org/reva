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
	"strings"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/pkg/errors"
	"google.golang.org/grpc"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/invite"
	"github.com/cs3org/reva/v3/pkg/ocm/invite/repository/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	revaservice "github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
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
	Driver            string                    `mapstructure:"driver"`
	Drivers           map[string]map[string]any `mapstructure:"drivers"`
	TokenExpiration   string                    `mapstructure:"token_expiration"`
	OCMClientTimeout  int                       `mapstructure:"ocm_timeout"`
	OCMClientInsecure bool                      `mapstructure:"ocm_insecure"`
	GatewaySVC        string                    `mapstructure:"gatewaysvc"                                    validate:"required"`
	ProviderDomain    string                    `docs:"The same domain registered in the provider authorizer" mapstructure:"provider_domain" validate:"required"`

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
func New(ctx context.Context, m map[string]any) (rgrpc.Service, error) {
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

// errMissingUser is the constant diagnostic for protected calls whose context
// identity is absent or malformed. It deliberately carries no request data.
var errMissingUser = errors.New("user not found in context")

// contextUser returns the context identity for protected calls. A present
// but broken identity (nil user or blank id) is rejected the same way as a
// missing one; callers must not fall back to untrusted request data.
func contextUser(ctx context.Context) (*userpb.User, error) {
	user, ok := appctx.ContextGetUser(ctx)
	if !ok || user == nil {
		return nil, errMissingUser
	}
	if err := invite.ValidateUserID(user.GetId(), "context user id"); err != nil {
		return nil, errMissingUser
	}
	return user, nil
}

func (s *service) GenerateInviteToken(ctx context.Context, req *invitepb.GenerateInviteTokenRequest) (*invitepb.GenerateInviteTokenResponse, error) {
	if req == nil {
		return &invitepb.GenerateInviteTokenResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	user, err := contextUser(ctx)
	if err != nil {
		return &invitepb.GenerateInviteTokenResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found in context"),
		}, nil
	}
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
	if req == nil {
		return &invitepb.ListInviteTokensResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	user, err := contextUser(ctx)
	if err != nil {
		return &invitepb.ListInviteTokensResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found in context"),
		}, nil
	}
	tokens, err := s.repo.ListTokens(ctx, user.GetId())
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

	ocmEndpoint, err := GetOCMEndpoint(req.GetOriginSystemProvider())
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

	// Normalize the remote user id against the origin provider domain: some
	// peers return a fully-qualified userID ("id@host" or "id@https://host")
	// which would otherwise be re-qualified into "id@host@host" downstream.
	providerDomain := req.GetOriginSystemProvider().Domain
	remoteUserID := &userpb.UserId{
		Type:     userpb.UserType_USER_TYPE_FEDERATED,
		Idp:      ocmd.TrimOCMScheme(providerDomain),
		OpaqueId: ocmd.NormalizeRemoteUserID(remoteUser.UserID, providerDomain),
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

func GetOCMEndpoint(originProvider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range originProvider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Endpoint.Path, nil
		}
	}
	return "", errors.New("ocm endpoint not specified for mesh provider")
}

func (s *service) AcceptInvite(ctx context.Context, req *invitepb.AcceptInviteRequest) (*invitepb.AcceptInviteResponse, error) {
	if req == nil {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	// the request's invite token only needs its secret; UserId and
	// Expiration are optional accompanying metadata
	if req.GetInviteToken() == nil || strings.TrimSpace(req.GetInviteToken().GetToken()) == "" {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInvalidArg(ctx, "missing invite token"),
		}, nil
	}
	if err := invite.ValidateRemoteUser(req.GetRemoteUser()); err != nil {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInvalidArg(ctx, "invalid remote user"),
		}, nil
	}

	token, err := s.repo.GetToken(ctx, req.GetInviteToken().GetToken())
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

	// the stored token is untrusted: validate the core record and the
	// expiry before resolving the initiator
	if invite.ValidateInviteToken(token) != nil || !isTokenValid(token) {
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

	remoteUser := req.GetRemoteUser()
	ocmd.CanonicalizeRemoteUserID(remoteUser.GetId())
	if err := invite.ValidateUserID(remoteUser.GetId(), "remote user id"); err != nil {
		return &invitepb.AcceptInviteResponse{
			Status: status.NewInvalidArg(ctx, "invalid remote user"),
		}, nil
	}

	if err := s.repo.AddRemoteUser(ctx, token.GetUserId(), remoteUser); err != nil {
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
	gw, err := revaservice.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	res, err := gw.GetUser(ctx, &userpb.GetUserRequest{
		UserId: id,
	})
	if err != nil {
		return nil, err
	}
	if res == nil || res.Status == nil {
		return nil, errors.New("missing user info response")
	}
	if res.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, errors.New(res.Status.Message)
	}
	if err := invite.ValidateRemoteUser(res.User); err != nil {
		return nil, err
	}

	return res.User, nil
}

func isTokenValid(token *invitepb.InviteToken) bool {
	if token == nil || token.Expiration == nil {
		return false
	}
	return time.Now().Unix() < int64(token.Expiration.Seconds)
}

func (s *service) GetAcceptedUser(ctx context.Context, req *invitepb.GetAcceptedUserRequest) (*invitepb.GetAcceptedUserResponse, error) {
	log := appctx.GetLogger(ctx)
	// TODO(lopresti): here we extract an opaque field to get the initiator of the invite, whereas we should implement
	// a GetRemoteUser() call in the repository that only takes the remoteUserId no matter the initiator.
	if req == nil {
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	if err := invite.ValidateUserID(req.GetRemoteUserId(), "remote user id"); err != nil {
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "invalid remote user id"),
		}, nil
	}
	user, ok := getUserFilter(ctx, req)
	if !ok {
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "invalid user filter"),
		}, nil
	}

	remoteUser, err := s.repo.GetRemoteUser(ctx, user.GetId(), req.GetRemoteUserId())
	if err != nil {
		log.Error().Err(err).Str("initiator", user.Id.OpaqueId).Any("remoteUser", req.GetRemoteUserId()).Msg("failed to look for OCM user")
		return &invitepb.GetAcceptedUserResponse{
			Status: status.NewStatusFromErrType(ctx, "error fetching remote user details", err),
		}, nil
	}
	if err := invite.ValidateRemoteUser(remoteUser); err != nil {
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
	// a present but broken context identity is not a signal to fall back to
	// the untrusted opaque payload
	user, ok := appctx.ContextGetUser(ctx)
	if ok {
		if user == nil || invite.ValidateUserID(user.GetId(), "context user id") != nil {
			return nil, false
		}
		return user, true
	}

	if req == nil || req.Opaque == nil || req.Opaque.Map == nil {
		return nil, false
	}

	v, ok := req.Opaque.Map["user-filter"]
	if !ok || v == nil {
		return nil, false
	}

	var u userpb.UserId
	if err := utils.UnmarshalJSONToProtoV1(v.Value, &u); err != nil {
		return nil, false
	}
	if err := invite.ValidateUserID(&u, "user filter id"); err != nil {
		return nil, false
	}
	return &userpb.User{Id: &u}, true
}

func (s *service) FindAcceptedUsers(ctx context.Context, req *invitepb.FindAcceptedUsersRequest) (*invitepb.FindAcceptedUsersResponse, error) {
	if req == nil {
		return &invitepb.FindAcceptedUsersResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	user, err := contextUser(ctx)
	if err != nil {
		return &invitepb.FindAcceptedUsersResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found in context"),
		}, nil
	}
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
	if req == nil {
		return &invitepb.DeleteAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "missing request"),
		}, nil
	}
	user, err := contextUser(ctx)
	if err != nil {
		return &invitepb.DeleteAcceptedUserResponse{
			Status: status.NewUnauthenticated(ctx, err, "user not found in context"),
		}, nil
	}
	if err := invite.ValidateUserID(req.GetRemoteUserId(), "remote user id"); err != nil {
		return &invitepb.DeleteAcceptedUserResponse{
			Status: status.NewInvalidArg(ctx, "invalid remote user id"),
		}, nil
	}
	if err := s.repo.DeleteRemoteUser(ctx, user.GetId(), req.GetRemoteUserId()); err != nil {
		return &invitepb.DeleteAcceptedUserResponse{
			Status: status.NewInternal(ctx, err, "error deleting remote users: "+err.Error()),
		}, nil
	}

	return &invitepb.DeleteAcceptedUserResponse{
		Status: status.NewOK(ctx),
	}, nil
}
