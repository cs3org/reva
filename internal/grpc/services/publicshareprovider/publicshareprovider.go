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

package publicshareprovider

import (
	"context"
	"regexp"

	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/publicshare"
	"github.com/cs3org/reva/v3/pkg/publicshare/manager/registry"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("publicshareprovider", New)
	plugin.RegisterNamespace("grpc.services.publicshareprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver                string                            `mapstructure:"driver"`
	Drivers               map[string]map[string]interface{} `mapstructure:"drivers"`
	AllowedPathsForShares []string                          `mapstructure:"allowed_paths_for_shares"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

type service struct {
	conf                  *config
	sm                    publicshare.Manager
	allowedPathsForShares []*regexp.Regexp
}

func getShareManager(ctx context.Context, c *config) (publicshare.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// TODO(labkode): add ctx to Close.
func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.sharing.link.v1beta1.LinkAPI/GetPublicShareByToken"}
}

func (s *service) Register(ss *grpc.Server) {
	link.RegisterLinkAPIServer(ss, s)
}

// New creates a new user share provider svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	sm, err := getShareManager(ctx, &c)
	if err != nil {
		return nil, err
	}

	allowedPathsForShares := make([]*regexp.Regexp, 0, len(c.AllowedPathsForShares))
	for _, s := range c.AllowedPathsForShares {
		regex, err := regexp.Compile(s)
		if err != nil {
			return nil, err
		}
		allowedPathsForShares = append(allowedPathsForShares, regex)
	}

	service := &service{
		conf:                  &c,
		sm:                    sm,
		allowedPathsForShares: allowedPathsForShares,
	}

	return service, nil
}

func (s *service) isPathAllowed(path string) bool {
	if len(s.allowedPathsForShares) == 0 {
		return true
	}
	for _, reg := range s.allowedPathsForShares {
		if reg.MatchString(path) {
			return true
		}
	}
	return false
}

func (s *service) CreatePublicShare(ctx context.Context, req *link.CreatePublicShareRequest) (*link.CreatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "create").Msg("create public share")

	if !s.isPathAllowed(req.ResourceInfo.Path) {
		return &link.CreatePublicShareResponse{
			Status: status.NewInvalidArg(ctx, "share creation is not allowed for the specified path"),
		}, nil
	}

	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	share, err := s.sm.CreatePublicShare(ctx, u, req.ResourceInfo, req.Grant, req.Description, req.Internal, req.NotifyUploads, req.NotifyUploadsExtraRecipients)
	switch err.(type) {
	case nil:
		return &link.CreatePublicShareResponse{
			Status: status.NewOK(ctx),
			Share:  share,
		}, nil
	case errtypes.NotFound:
		return &link.CreatePublicShareResponse{
			Status: status.NewNotFound(ctx, "resource does not exist"),
		}, nil
	case errtypes.AlreadyExists:
		return &link.CreatePublicShareResponse{
			Status: status.NewAlreadyExists(ctx, err, "share already exists"),
		}, nil
	default:
		return &link.CreatePublicShareResponse{
			Status: status.NewInternal(ctx, err, "unknown error"),
		}, nil
	}
}

func (s *service) RemovePublicShare(ctx context.Context, req *link.RemovePublicShareRequest) (*link.RemovePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "remove").Msg("remove public share")

	user := appctx.ContextMustGetUser(ctx)
	err := s.sm.RevokePublicShare(ctx, user, req.Ref)
	switch err.(type) {
	case nil:
		return &link.RemovePublicShareResponse{
			Status: status.NewOK(ctx),
		}, nil
	case errtypes.NotFound:
		return &link.RemovePublicShareResponse{
			Status: status.NewNotFound(ctx, "unknown token"),
		}, nil
	default:
		return &link.RemovePublicShareResponse{
			Status: status.NewInternal(ctx, err, "error deleting public share"),
		}, nil
	}
}

func (s *service) GetPublicShareByToken(ctx context.Context, req *link.GetPublicShareByTokenRequest) (*link.GetPublicShareByTokenResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Debug().Msg("getting public share by token")

	// there are 2 passes here, and the second request has no password
	found, err := s.sm.GetPublicShareByToken(ctx, req.GetToken(), req.GetAuthentication(), req.GetSign())
	switch v := err.(type) {
	case nil:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewOK(ctx),
			Share:  found,
		}, nil
	case errtypes.InvalidCredentials:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewPermissionDenied(ctx, v, "wrong password"),
		}, nil
	case errtypes.NotFound:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewNotFound(ctx, "unknown token"),
		}, nil
	default:
		return &link.GetPublicShareByTokenResponse{
			Status: status.NewInternal(ctx, v, "unexpected error"),
		}, nil
	}
}

func (s *service) GetPublicShare(ctx context.Context, req *link.GetPublicShareRequest) (*link.GetPublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "get").Msg("get public share")

	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	found, err := s.sm.GetPublicShare(ctx, u, req.Ref, req.GetSign())
	switch err.(type) {
	case nil:
		return &link.GetPublicShareResponse{
			Status: status.NewOK(ctx),
			Share:  found,
		}, nil
	case errtypes.NotFound:
		return &link.GetPublicShareResponse{
			Status: status.NewNotFound(ctx, "share not found"),
		}, nil
	default:
		return &link.GetPublicShareResponse{
			Status: status.NewInternal(ctx, err, "unknown error"),
		}, nil
	}
}

func (s *service) ListPublicShares(ctx context.Context, req *link.ListPublicSharesRequest) (*link.ListPublicSharesResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "list").Msg("list public share")
	user, _ := appctx.ContextGetUser(ctx)

	if req.Opaque != nil {
		if v, ok := req.Opaque.Map[appctx.ResoucePathCtx]; ok {
			ctx = appctx.ContextSetResourcePath(ctx, string(v.Value))
		}
	}

	shares, err := s.sm.ListPublicShares(ctx, user, req.Filters, &provider.ResourceInfo{}, req.GetSign())
	if err != nil {
		log.Err(err).Msg("error listing shares")
		return &link.ListPublicSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing public shares"),
		}, nil
	}

	res := &link.ListPublicSharesResponse{
		Status: status.NewOK(ctx),
		Share:  shares,
	}
	return res, nil
}

func (s *service) UpdatePublicShare(ctx context.Context, req *link.UpdatePublicShareRequest) (*link.UpdatePublicShareResponse, error) {
	log := appctx.GetLogger(ctx)
	log.Info().Str("publicshareprovider", "update").Msg("update public share")

	u, ok := appctx.ContextGetUser(ctx)
	if !ok {
		log.Error().Msg("error getting user from context")
	}

	updated, err := s.sm.UpdatePublicShare(ctx, u, req, nil)
	switch err.(type) {
	case nil:
		return &link.UpdatePublicShareResponse{
			Status: status.NewOK(ctx),
			Share:  updated,
		}, nil
	case errtypes.NotFound:
		return &link.UpdatePublicShareResponse{
			Status: status.NewNotFound(ctx, "share not found"),
		}, nil
	default:
		return &link.UpdatePublicShareResponse{
			Status: status.NewInternal(ctx, err, "unknown error"),
		}, nil
	}
}
