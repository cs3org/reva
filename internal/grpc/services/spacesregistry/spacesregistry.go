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
	"fmt"
	"math"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/projects/manager/registry"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/cs3org/reva/v3/pkg/utils/list"
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
	// Provide a list of public spaces, where we map
	// name:
	//  - path: <path>
	//  - description: <description>
	PublicSpaces map[string]map[string]string `mapstructure:"public_spaces"`
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
	// The creation of a space requires a provisioning and approval workflow, which for now is implemented externally
	return nil, errors.New("not supportedd")
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

		publicSpaces, err := s.getPublicSpaces(ctx)
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
		}
		sp = append(sp, publicSpaces...)
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

	switch spaceType {
	case spaces.SpaceTypeHome:
		space, err := s.userSpace(ctx, user)
		if err != nil {
			return nil, err
		}
		if space != nil {
			sp = append(sp, space)
		}
	case spaces.SpaceTypeProject:
		resp, err := s.projects.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{})
		if err != nil {
			return nil, err
		}
		if resp.Status.Code != rpcv1beta1.Code_CODE_OK {
			return nil, fmt.Errorf("%s: %s", resp.Status.Code.String(), resp.Status.Message)
		}

		projects := resp.StorageSpaces
		if err := s.decorateProjects(ctx, projects); err != nil {
			return nil, err
		}
		sp = append(sp, projects...)

		// We also want public spaces when you search for projects, because this filtering
		// happens in the front-end
		fallthrough

	case spaces.SpaceTypePublic:
		publicSpaces, err := s.getPublicSpaces(ctx)
		if err != nil {
			return nil, err
		}

		sp = append(sp, publicSpaces...)
	}

	return sp, nil
}

func (s *service) decorateProjects(ctx context.Context, projects []*provider.StorageSpace) error {
	for _, proj := range projects {
		// Add quota

		// To get the quota for a project, we cannot do the request
		// on behalf of the current logged user, because the project
		// is owned by an other account, in general different from the
		// logged in user.
		// We need then to impersonate the owner and ask the quota
		// on behalf of him.

		// This is no longer necessary for the new project quota nodes,
		// but we need to keep it here until we migrate all of the old
		// project quota nodes
		// See CERNBOX-3995

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

		ownerCtx := appctx.ContextSetToken(context.Background(), token)
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

		// Add mtime of space
		statRes, err := s.gw.Stat(ctx, &provider.StatRequest{
			Ref: &provider.Reference{
				Path: proj.RootInfo.Path,
			},
		})
		if err != nil {
			return err
		}
		if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return errors.New(statRes.Status.Message)
		}

		proj.Mtime = statRes.Info.Mtime
	}
	return nil
}

func (s *service) userSpace(ctx context.Context, user *userpb.User) (*provider.StorageSpace, error) {
	if user.Id.Type == userpb.UserType_USER_TYPE_FEDERATED || user.Id.Type == userpb.UserType_USER_TYPE_LIGHTWEIGHT {
		return nil, nil // lightweight and federated accounts are not eligible for a user space
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
	if stat.Status.Code != rpcv1beta1.Code_CODE_OK {
		return nil, fmt.Errorf("Failed to stat %s: got status %s with message: %s", home, stat.Status.GetCode().String(), stat.Status.GetMessage())
	}

	quota, err := s.gw.GetQuota(ctx, &gateway.GetQuotaRequest{
		Ref: &provider.Reference{
			Path: home,
		},
	})
	if err != nil {
		return nil, err
	}

	if stat.Info == nil || stat.Info.Id == nil || stat.Info.Id.StorageId == "" {
		return nil, errors.New("Received an invalid storageID")
	}

	return &provider.StorageSpace{
		Id: &provider.StorageSpaceId{
			OpaqueId: spaces.EncodeStorageSpaceID(stat.Info.Id.StorageId, home),
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

func (s *service) getPublicSpaces(ctx context.Context) ([]*provider.StorageSpace, error) {
	log := appctx.GetLogger(ctx)
	publicSpaces := make([]*provider.StorageSpace, 0)
	for spaceName, content := range s.c.PublicSpaces {
		path, ok := content["path"]
		if !ok {
			log.Error().Msgf("No `path` found for public space %s, ignoring this space", spaceName)
			continue
		}

		statRes, err := s.gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{
			Path: path,
		}})
		if err != nil || statRes.Status == nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			log.Error().Err(err).Any("Status", statRes.Status).Msgf("Failed to stat path %s for public space %s, ignoring this space", path, spaceName)
			continue
		}

		spaceID := spaces.EncodeSpaceID(path)
		space := &provider.StorageSpace{
			SpaceType: "public",
			Root:      statRes.Info.Id,
			Id: &provider.StorageSpaceId{
				OpaqueId: spaceID,
			},
			RootInfo:        statRes.Info,
			Mtime:           statRes.Info.Mtime,
			Name:            spaceName,
			HasTrashedItems: false,
			Quota: &provider.Quota{
				// 1 Exabyte
				QuotaMaxBytes:  uint64(math.Pow10(18)),
				RemainingBytes: uint64(math.Pow10(18)) - statRes.Info.Size,
			},
		}

		if description, ok := content["description"]; ok {
			space.Description = description
		}

		publicSpaces = append(publicSpaces, space)
	}
	return publicSpaces, nil
}

func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return s.projects.UpdateStorageSpace(ctx, req)
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	// As for the creation, the deletion of a space is implemented externally for now
	return nil, errors.New("not supported")
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterSpacesAPIServer(ss, s)
}

func (s *service) UnprotectedEndpoints() []string { return nil }

func (s *service) Close() error { return nil }
