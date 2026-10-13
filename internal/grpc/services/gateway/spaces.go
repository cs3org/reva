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

package gateway

import (
	"context"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

func (s *svc) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	c, err := service.Spaces(ctx)
	if err != nil {
		return &provider.CreateStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.CreateStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling CreateStorageSpace")
	}

	return res, nil
}

func (s *svc) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	c, err := service.Spaces(ctx)
	if err != nil {
		return &provider.ListStorageSpacesResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.ListStorageSpaces(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling ListStorageSpaces")
	}

	return res, nil
}

// addSpaceRoles adds to info the permissions that the roles of its project
// space give to the user in the context.
func (s *svc) addSpaceRoles(ctx context.Context, info *provider.ResourceInfo) {
	if info.GetSpace().GetSpaceType() != spaces.SpaceTypeProject.AsString() || info.GetSpace().GetId() == nil || info.GetPermissionSet() == nil {
		return
	}
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return
	}

	space, err := s.fetchSpace(ctx, info.Space.Id.OpaqueId)
	if err != nil {
		appctx.GetLogger(ctx).Error().Err(err).Str("space_id", info.Space.Id.OpaqueId).Msg("gateway: failed to fetch space, its roles are not applied")
		return
	}
	spaces.MergeRolesWithPermissions(info.PermissionSet, space.GetRoles(), user)
}

// fetchSpace returns the project space with the given ID. A space holds
// nothing that depends on the user, so all users share the cached entry.
func (s *svc) fetchSpace(ctx context.Context, spaceID string) (*provider.StorageSpace, error) {
	if space, err := s.spaceInfoCache.Get(spaceID); err == nil {
		return space, nil
	}

	res, err := s.ListStorageSpaces(ctx, &provider.ListStorageSpacesRequest{
		Filters: spaces.ListStorageSpaceFilter{}.ByID(&provider.StorageSpaceId{OpaqueId: spaceID}).List(),
	})
	if err != nil {
		return nil, err
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		return nil, errors.Errorf("gateway: failed to list space %s: %s", spaceID, res.Status.Message)
	}
	if len(res.StorageSpaces) != 1 {
		return nil, errtypes.NotFound(spaceID)
	}

	space := res.StorageSpaces[0]
	_ = s.spaceInfoCache.SetWithExpire(spaceID, space, s.spaceInfoCacheTTL)
	return space, nil
}

func (s *svc) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	c, err := service.Spaces(ctx)
	if err != nil {
		return &provider.UpdateStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.UpdateStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling UpdateStorageSpace")
	}

	return res, nil
}

func (s *svc) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	c, err := service.Spaces(ctx)
	if err != nil {
		return &provider.DeleteStorageSpaceResponse{
			Status: status.NewInternal(ctx, err, "error getting spaces client"),
		}, nil
	}

	res, err := c.DeleteStorageSpace(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "gateway: error calling DeleteStorageSpace")
	}

	return res, nil
}
