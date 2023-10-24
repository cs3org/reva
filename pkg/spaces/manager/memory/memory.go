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

	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/spaces/manager/registry"
	"github.com/cs3org/reva/pkg/storage/utils/templates"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
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
	UserSpace string             `mapstructure:"user_space"`
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

func (s *service) ListSpaces(ctx context.Context, user *userpb.User) ([]*provider.StorageSpace, error) {
	sp := []*provider.StorageSpace{}

	// home space
	if space := s.userSpace(ctx, user); space != nil {
		sp = append(sp, space)
	}

	// project spaces
	projects := s.projectSpaces(ctx, user)
	sp = append(sp, projects...)

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
			Path: path,
		},
	}
}

func (s *service) projectSpaces(ctx context.Context, user *userpb.User) []*provider.StorageSpace {
	projects := []*provider.StorageSpace{}
	for _, space := range s.c.Spaces {
		if projectBelongToUser(user, &space) {
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
					Path: space.Path,
				},
			})
		}
	}
	return projects
}

func projectBelongToUser(user *userpb.User, project *SpaceDescription) bool {
	return user.Id.OpaqueId == project.Owner ||
		slices.Contains(user.Groups, project.Admins) ||
		slices.Contains(user.Groups, project.Readers) ||
		slices.Contains(user.Groups, project.Writers)
}

func (s *service) UpdateSpace(ctx context.Context, space *provider.StorageSpace) error {
	return errors.New("not yet implemented")
}

func (s *service) DeleteSpace(ctx context.Context, spaceID *provider.StorageSpaceId) error {
	return errors.New("not yet implemented")
}
