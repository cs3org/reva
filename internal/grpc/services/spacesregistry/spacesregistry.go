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
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	cachereg "github.com/cs3org/reva/v3/pkg/share/cache/registry"

	"github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/projects/manager/registry"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/v3/pkg/share/cache"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"github.com/rs/zerolog/log"
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
	PublicSpaces             map[string]map[string]string      `mapstructure:"public_spaces"`
	ResourceInfoCacheDrivers map[string]map[string]interface{} `mapstructure:"resource_info_caches"`
	ResourceInfoCacheDriver  string                            `mapstructure:"resource_info_cache_type"`
	ResourceInfoCacheTTL     int                               `mapstructure:"resource_info_cache_ttl"`
}

func (c *config) ApplyDefaults() {
	if c.UserSpace == "" {
		c.UserSpace = "/home"
	}
}

type service struct {
	c                    *config
	projects             projects.Catalogue
	gw                   gateway.GatewayAPIClient
	resourceInfoCache    cache.ResourceInfoCache
	resourceInfoCacheTTL time.Duration
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

	ricache, err := getCacheManager(&c)
	if err == nil {
		svc.resourceInfoCache = ricache
		svc.resourceInfoCacheTTL = time.Second * time.Duration(c.ResourceInfoCacheTTL)
	}

	return &svc, nil
}

func getCacheManager(c *config) (cache.ResourceInfoCache, error) {
	factory, err := cachereg.GetCacheFunc[cache.ResourceInfoCache]("memory")
	if err != nil {
		return nil, err
	}
	return factory(c.ResourceInfoCacheDrivers[c.ResourceInfoCacheDriver])
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
		homes, err := s.listSpacesByType(ctx, req, user, spaces.SpaceTypeHome)
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
		}
		sp = append(sp, homes...)

		projects, err := s.listSpacesByType(ctx, req, user, spaces.SpaceTypeProject)
		if projects != nil {
			sp = append(sp, projects...)
		}
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error()), StorageSpaces: sp}, nil
		}

		publicSpaces, err := s.getPublicSpaces(ctx)
		if err != nil {
			return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
		}
		sp = append(sp, publicSpaces...)
	} else {
		// Here, we only check for the SpaceType filter
		// Other filters are handled at the driver level
		for _, filter := range filters {
			switch filter.Type {
			case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
				spaces, err := s.listSpacesByType(ctx, req, user, spaces.SpaceType(filter.Term.(*provider.ListStorageSpacesRequest_Filter_SpaceType).SpaceType))
				if err != nil {
					return &provider.ListStorageSpacesResponse{Status: status.NewInternal(ctx, err, err.Error())}, nil
				}
				sp = append(sp, spaces...)
			}
		}
	}

	return &provider.ListStorageSpacesResponse{Status: status.NewOK(ctx), StorageSpaces: sp}, nil
}

func (s *service) listSpacesByType(ctx context.Context, req *provider.ListStorageSpacesRequest, user *userpb.User, spaceType spaces.SpaceType) ([]*provider.StorageSpace, error) {
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
		log.Debug().Msg("Listing spaces by type project")
		resp, err := s.projects.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
			Filters: req.Filters,
		})
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

		// For now, we also return public spaces when you query for projects
		// as the front-end will filter these
		// but only if the request was not made with an ID-filter,
		// as that would mean the requestor is looking for a specific space
		//
		// Having a `fallthrough` here would've been nice, but Go does
		// not allow conditional fallthroughs
		if _, isFilterById := isFilterByID(req.Filters); !isFilterById {
			publicSpaces, err := s.getPublicSpaces(ctx)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to get public spaces in call to listSpacesByType with type project")
				return sp, err
			}

			sp = append(sp, publicSpaces...)
		}

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
	log := appctx.GetLogger(ctx)
	for _, proj := range projects {
		err := s.decorateProject(ctx, proj)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to decorate project %s", proj.Name)
		}
	}
	return nil
}

func (s *service) decorateProject(ctx context.Context, proj *provider.StorageSpace) error {
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

	log.Debug().Msgf("Fetching quota for project %s", proj.Name)
	quota, err := s.gw.GetQuota(ownerCtx, &gateway.GetQuotaRequest{
		Ref: &provider.Reference{
			Path: proj.RootInfo.Path,
		},
	})
	if err != nil {
		log.Err(err).Msgf("Failed to fetch quota for project %s", proj.Name)
		return err
	}
	proj.Quota = &provider.Quota{
		QuotaMaxBytes:  quota.TotalBytes,
		RemainingBytes: quota.TotalBytes - quota.UsedBytes,
	}

	// Add mtime of space
	var resourceInfo *provider.ResourceInfo
	if res, err := s.resourceInfoCache.Get(proj.RootInfo.Path); err == nil && res != nil {
		resourceInfo = res
	} else {
		statRes, err := s.gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{
			Path: proj.RootInfo.Path,
		}})
		if err != nil || statRes.Status == nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
			return fmt.Errorf("failed to stat path %s for project %s", proj.RootInfo.Path, proj.Name)
		}
		resourceInfo = statRes.Info
		s.resourceInfoCache.Set(proj.RootInfo.Path, resourceInfo)
	}

	proj.Mtime = resourceInfo.Mtime
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
		return nil, fmt.Errorf("failed to stat %s: got status %s with message: %s", home, stat.Status.GetCode().String(), stat.Status.GetMessage())
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
		return nil, errors.New("received an invalid storageID")
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
		PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
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

		var resourceInfo *provider.ResourceInfo
		if res, err := s.resourceInfoCache.Get(path); err == nil && res != nil {
			resourceInfo = res
		} else {
			statRes, err := s.gw.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{
				Path: path,
			}})
			if err != nil || statRes.Status == nil || statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
				log.Error().Err(err).Any("Status", statRes.Status).Msgf("Failed to stat path %s for public space %s, ignoring this space", path, spaceName)
			} else {
				resourceInfo = statRes.Info
				s.resourceInfoCache.Set(path, resourceInfo)
			}
		}

		if resourceInfo == nil {
			continue
		}

		spaceID := spaces.EncodeStorageSpaceID(resourceInfo.Id.StorageId, path)
		space := &provider.StorageSpace{
			SpaceType: spaces.SpaceTypePublic.AsString(),
			Root:      resourceInfo.Id,
			Id: &provider.StorageSpaceId{
				OpaqueId: spaceID,
			},
			RootInfo:        resourceInfo,
			Mtime:           resourceInfo.Mtime,
			Name:            spaceName,
			HasTrashedItems: false,
			Quota: &provider.Quota{
				// 1 Exabyte
				QuotaMaxBytes:  uint64(math.Pow10(18)),
				RemainingBytes: uint64(math.Pow10(18)) - resourceInfo.Size,
			},
		}

		if description, ok := content["description"]; ok {
			space.Description = description
		}
		if thumbnailPath, ok := content["thumbnail_path"]; ok {
			space.ThumbnailId = thumbnailPath
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

func isFilterByID(filters []*provider.ListStorageSpacesRequest_Filter) (string, bool) {
	for _, f := range filters {
		if f.Type == provider.ListStorageSpacesRequest_Filter_TYPE_ID {
			return f.Term.(*provider.ListStorageSpacesRequest_Filter_Id).Id.OpaqueId, true
		}
	}
	return "", false
}

func (s *service) Register(ss *grpc.Server) {
	provider.RegisterSpacesAPIServer(ss, s)
}

func (s *service) UnprotectedEndpoints() []string { return nil }

func (s *service) Close() error { return nil }
