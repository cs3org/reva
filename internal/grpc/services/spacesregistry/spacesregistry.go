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

package spacesregistry

import (
	"context"
	"errors"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/projects"
	"github.com/cs3org/reva/pkg/projects/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/cs3org/reva/pkg/utils/list"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func init() {
	rgrpc.Register("spacesregistry", New)
	plugin.RegisterNamespace("grpc.services.spacesregistry.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver        string                    `mapstructure:"driver"`
	Drivers       map[string]map[string]any `mapstructure:"drivers"`
	UserSpace     string                    `mapstructure:"user_space" validate:"required"`
	MachineSecret string                    `mapstructure:"machine_secret" validate:"required"`
}

func (c *config) ApplyDefaults() {
	if c.UserSpace == "" {
		c.UserSpace = "/home"
	}
}

type service struct {
	c        *config
	projects projects.Catalogue
	gw       gateway.GatewayAPIClient
}

func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	s, err := getSpacesDriver(ctx, c.Driver, c.Drivers)
	if err != nil {
		return nil, err
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(sharedconf.GetGatewaySVC("")))
	if err != nil {
		return nil, err
	}

	svc := service{
		c:        &c,
		projects: s,
		gw:       client,
	}
	return &svc, nil
}

func getSpacesDriver(ctx context.Context, driver string, cfg map[string]map[string]any) (projects.Catalogue, error) {
	if f, ok := registry.NewFuncs[driver]; ok {
		return f(ctx, cfg[driver])
	}
	return nil, errtypes.NotFound("driver not found: " + driver)
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func countTypeFilters(filters []*provider.ListStorageSpacesRequest_Filter) (count int) {
	for _, f := range filters {
		if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE {
			count++
		}
	}
	return
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
	filters := req.Filters

	sp := []*provider.StorageSpace{}
	if countTypeFilters(filters) == 0 {
		homes, err := s.listSpacesByType(ctx, user, spaces.SpaceTypeHome)
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
		}
		sp = append(sp, homes...)

		projects, err := s.listSpacesByType(ctx, user, spaces.SpaceTypeProject)
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
		}
		sp = append(sp, projects...)
	}

	for _, filter := range filters {
		switch filter.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			spaces, err := s.listSpacesByType(ctx, user, spaces.SpaceType(filter.Term.(*provider.ListStorageSpacesRequest_Filter_SpaceType).SpaceType))
			if err != nil {
				return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
			}
			sp = append(sp, spaces...)
		case provider.ListStorageSpacesRequest_Filter_TYPE_ID:
		default:
			return nil, errtypes.NotSupported("filter not supported")
		}
	}

	// TODO: we should filter at the driver level.
	// for now let's do it here. optimizations later :)
	if id, ok := isFilterByID(req.Filters); ok {
		sp = list.Filter(sp, func(s *provider.StorageSpace) bool { return s.Id.OpaqueId == id })
	}

	return &provider.ListStorageSpacesResponse{Status: status.NewOK(ctx), StorageSpaces: sp}, nil
}

func isFilterByID(filters []*provider.ListStorageSpacesRequest_Filter) (string, bool) {
	for _, f := range filters {
		if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
			return f.Term.(*provider.ListStorageSpacesRequest_Filter_Id).Id.OpaqueId, true
		}
	}
	return "", false
}

func (s *service) listSpacesByType(ctx context.Context, user *userpb.User, spaceType spaces.SpaceType) ([]*provider.StorageSpace, error) {
	sp := []*provider.StorageSpace{}

	if spaceType == spaces.SpaceTypeHome {
		space, err := s.userSpace(ctx, user)
		if err != nil {
			return nil, err
		}
		if space != nil {
			sp = append(sp, space)
		}
	} else if spaceType == spaces.SpaceTypeProject {
		projects, err := s.projects.ListProjects(ctx, user)
		if err != nil {
			return nil, err
		}
		if err := s.addQuotaToProjects(ctx, projects); err != nil {
			return nil, err
		}
		sp = append(sp, projects...)
	}

	return sp, nil
}

func (s *service) addQuotaToProjects(ctx context.Context, projects []*provider.StorageSpace) error {
	for _, proj := range projects {
		// To get the quota for a project, we cannot do the request
		// on behalf of the current logged user, because the project
		// is owned by an other account, in general different from the
		// logged in user.
		// We need then to impersonate the owner and ask the quota
		// on behalf of him.

		authRes, err := s.gw.Authenticate(ctx, &gateway.AuthenticateRequest{
			Type:         "machine",
			ClientId:     proj.Owner.Id.OpaqueId,
			ClientSecret: s.c.MachineSecret,
		})
		if err != nil {
			return err
		}
		if authRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return errors.New(authRes.Status.Message)
		}

		token := authRes.Token
		owner := authRes.User

		ownerCtx := appctx.ContextSetToken(context.TODO(), token)
		ownerCtx = metadata.AppendToOutgoingContext(ownerCtx, appctx.TokenHeader, token)
		ownerCtx = appctx.ContextSetUser(ownerCtx, owner)

		quota, err := s.gw.GetQuota(ownerCtx, &gateway.GetQuotaRequest{
			Ref: &provider.Reference{
				Path: proj.RootInfo.Path,
			},
		})
		if err != nil {
			return err
		}
		proj.Quota = &provider.Quota{
			QuotaMaxBytes:  quota.TotalBytes,
			RemainingBytes: quota.TotalBytes - quota.UsedBytes,
		}
	}
	return nil
}

func (s *service) userSpace(ctx context.Context, user *userpb.User) (*provider.StorageSpace, error) {
	if utils.UserIsLightweight(user) {
		return nil, nil // lightweight accounts and federated do not have a user space
	}

	home := templates.WithUser(user, s.c.UserSpace) // TODO: we can use gw.GetHome() call
	stat, err := s.gw.Stat(ctx, &provider.StatRequest{
		Ref: &provider.Reference{
			Path: home,
		},
	})
	if err != nil {
		return nil, err
	}

	quota, err := s.gw.GetQuota(ctx, &gateway.GetQuotaRequest{
		Ref: &provider.Reference{
			Path: home,
		},
	})
	if err != nil {
		return nil, err
	}

	return &provider.StorageSpace{
		Id: &provider.StorageSpaceId{
			OpaqueId: spaces.EncodeSpaceID(stat.Info.Id.StorageId, home),
		},
		Owner:     user,
		Name:      user.Username,
		SpaceType: spaces.SpaceTypeHome.AsString(),
		RootInfo: &provider.ResourceInfo{
			PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
			Path:          home,
		},
		Quota: &provider.Quota{
			QuotaMaxBytes:  quota.TotalBytes,
			RemainingBytes: quota.TotalBytes - quota.UsedBytes,
		},
	}, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, errors.New("not yet implemented")
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterSpacesAPIServer(ss, s)
}

func (s *service) UnprotectedEndpoints() []string { return nil }

func (s *service) Close() error { return nil }
