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

package memory

import (
	"context"
	"errors"
	"slices"

	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/spaces/manager/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	conversions "github.com/cs3org/reva/internal/http/services/owncloud/ocs/conversions"
)

func init() {
	registry.Register("memory", New)
}

type SpaceDescription struct {
	ID      string `mapstructure:"id"      validate:"required"`
	Path    string `mapstructure:"path"    validate:"required"`
	Name    string `mapstructure:"name"    validate:"required"`
	Type    string `mapstructure:"type"    validate:"required"`
	Owner   string `mapstructure:"owner"   validate:"required"`
	Readers string `mapstructure:"readers" validate:"required"`
	Writers string `mapstructure:"writers" validate:"required"`
	Admins  string `mapstructure:"admins"  validate:"required"`
}

type Config struct {
	Spaces    []SpaceDescription `mapstructure:"spaces"`
	UserSpace string             `mapstructure:"user_space" validate:"required"`
}

func (c *Config) ApplyDefaults() {
	if c.UserSpace == "" {
		c.UserSpace = "/home"
	}
}

type service struct {
	c *Config
}

func New(ctx context.Context, m map[string]any) (spaces.Manager, error) {
	var c Config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	return NewWithConfig(ctx, &c)
}

func NewWithConfig(ctx context.Context, c *Config) (spaces.Manager, error) {
	return &service{c: c}, nil
}

func (s *service) StoreSpace(ctx context.Context, owner *userpb.UserId, path, name string, quota *provider.Quota) error {
	return errors.New("not yet implemented")
}

func (s *service) listSpacesByType(ctx context.Context, user *userpb.User, spaceType spaces.SpaceType) []*provider.StorageSpace {
	sp := []*provider.StorageSpace{}

	if spaceType == spaces.SpaceTypeHome {
		if space := s.userSpace(ctx, user); space != nil {
			sp = append(sp, space)
		}
	} else if spaceType == spaces.SpaceTypeProject {
		projects := s.projectSpaces(ctx, user)
		sp = append(sp, projects...)
	}

	return sp
}

func (s *service) ListSpaces(ctx context.Context, user *userpb.User, filters []*provider.ListStorageSpacesRequest_Filter) ([]*provider.StorageSpace, error) {
	sp := []*provider.StorageSpace{}
	if len(filters) == 0 {
		sp = s.listSpacesByType(ctx, user, spaces.SpaceTypeHome)
		sp = append(sp, s.listSpacesByType(ctx, user, spaces.SpaceTypeProject)...)
		return sp, nil
	}

	for _, filter := range filters {
		switch filter.Type {
		case provider.ListStorageSpacesRequest_Filter_TYPE_SPACE_TYPE:
			sp = append(sp, s.listSpacesByType(ctx, user, spaces.SpaceType(filter.Term.(*provider.ListStorageSpacesRequest_Filter_SpaceType).SpaceType))...)
		default:
			return nil, errtypes.NotSupported("filter not supported")
		}
	}
	return sp, nil
}

func (s *service) userSpace(ctx context.Context, user *userpb.User) *provider.StorageSpace {
	if utils.UserIsLightweight(user) {
		return nil // lightweight accounts and federated do not have a user space
	}
	path := templates.WithUser(user, s.c.UserSpace)
	return &provider.StorageSpace{
		Id: &provider.StorageSpaceId{
			OpaqueId: path,
		},
		Owner:     user,
		Name:      user.Username,
		SpaceType: spaces.SpaceTypeHome.AsString(),
		RootInfo: &provider.ResourceInfo{
			PermissionSet: conversions.NewManagerRole().CS3ResourcePermissions(),
			Path:          path,
		},
	}
}

func (s *service) projectSpaces(ctx context.Context, user *userpb.User) []*provider.StorageSpace {
	projects := []*provider.StorageSpace{}
	for _, space := range s.c.Spaces {
		if perms, ok := projectBelongToUser(user, &space); ok {
			projects = append(projects, &provider.StorageSpace{
				Id: &provider.StorageSpaceId{
					OpaqueId: space.ID,
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
	return projects
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

func (s *service) UpdateSpace(ctx context.Context, space *provider.StorageSpace) error {
	return errors.New("not yet implemented")
}

func (s *service) DeleteSpace(ctx context.Context, spaceID *provider.StorageSpaceId) error {
	return errors.New("not yet implemented")
}
