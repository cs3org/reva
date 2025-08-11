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

package memory

import (
	"context"
	"errors"
	"slices"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/projects"
	"github.com/cs3org/reva/v3/pkg/projects/manager/registry"
	"github.com/cs3org/reva/v3/pkg/spaces"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	conversions "github.com/cs3org/reva/v3/internal/http/services/owncloud/ocs/conversions"
)

func init() {
	registry.Register("memory", New)
}

type SpaceDescription struct {
	StorageID string `mapstructure:"storage_id" validate:"required"`
	Path      string `mapstructure:"path"       validate:"required"`
	Name      string `mapstructure:"name"       validate:"required"`
	Owner     string `mapstructure:"owner"      validate:"required"`
	Readers   string `mapstructure:"readers"    validate:"required"`
	Writers   string `mapstructure:"writers"    validate:"required"`
	Admins    string `mapstructure:"admins"     validate:"required"`
}

type Config struct {
	Spaces []SpaceDescription `mapstructure:"spaces"`
}

type service struct {
	c *Config
}

func New(ctx context.Context, m map[string]any) (projects.Catalogue, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return NewWithConfig(ctx, &c)
}

func NewWithConfig(ctx context.Context, c *Config) (projects.Catalogue, error) {
	return &service{c: c}, nil
}

func (s *service) ListStorageSpaces(ctx context.Context, req *provider.ListStorageSpacesRequest) (*provider.ListStorageSpacesResponse, error) {
	projects := []*provider.StorageSpace{}
	user, ok := appctx.ContextGetUser(ctx)
	if !ok {
		return &provider.ListStorageSpacesResponse{
			Status: &rpcv1beta1.Status{
				Code:    rpcv1beta1.Code_CODE_UNAUTHENTICATED,
				Message: "must provide a user for listing storage spaces",
			},
		}, nil
	}
	for _, space := range s.c.Spaces {
		if perms, ok := projectBelongToUser(user, &space); ok {
			projects = append(projects, &provider.StorageSpace{
				Id: &provider.StorageSpaceId{
					OpaqueId: spaces.EncodeStorageSpaceID(space.StorageID, space.Path),
				},
				Owner: &userpb.User{
					Id: &userpb.UserId{
						OpaqueId: space.Owner,
					},
				},
				Name:      space.Name,
				SpaceType: spaces.SpaceTypeProject.AsString(),
				RootInfo: &provider.ResourceInfo{
					Path:          space.Path,
					PermissionSet: perms,
				},
			})
		}
	}
	return &provider.ListStorageSpacesResponse{
		StorageSpaces: projects,
		Status: &rpcv1beta1.Status{
			Code: rpcv1beta1.Code_CODE_OK,
		},
	}, nil
}

// TODO: at least this should be implemented
func (s *service) UpdateStorageSpace(ctx context.Context, req *provider.UpdateStorageSpaceRequest) (*provider.UpdateStorageSpaceResponse, error) {
	return nil, errors.New("Unsupported")
}

func (s *service) CreateStorageSpace(ctx context.Context, req *provider.CreateStorageSpaceRequest) (*provider.CreateStorageSpaceResponse, error) {
	return nil, errors.New("Unsupported")
}

func (s *service) DeleteStorageSpace(ctx context.Context, req *provider.DeleteStorageSpaceRequest) (*provider.DeleteStorageSpaceResponse, error) {
	return nil, errors.New("Unsupported")
}

func projectBelongToUser(user *userpb.User, project *SpaceDescription) (*provider.ResourcePermissions, bool) {
	if user.Id.OpaqueId == project.Owner {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, project.Admins) {
		return conversions.NewManagerRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, project.Writers) {
		return conversions.NewEditorRole().CS3ResourcePermissions(), true
	}
	if slices.Contains(user.Groups, project.Readers) {
		return conversions.NewViewerRole().CS3ResourcePermissions(), true
	}
	return nil, false
}

var _ projects.Catalogue = (*service)(nil)
