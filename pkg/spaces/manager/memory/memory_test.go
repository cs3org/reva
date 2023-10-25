// Copyright 2018-2023 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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

package memory_test

import (
	"context"
	"slices"
	"testing"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/spaces"
	"github.com/cs3org/reva/pkg/spaces/manager/memory"
	"github.com/cs3org/reva/pkg/utils"
)

var einstein = &userpb.User{
	Id: &userpb.UserId{
		Idp:      "example.org",
		OpaqueId: "einstein",
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	},
	Username: "einstein",
	Groups:   []string{"cernbox-projects-cernbox-admins", "violin-haters", "physics-lovers"},
}

var marie = &userpb.User{
	Id: &userpb.UserId{
		Idp:      "example.org",
		OpaqueId: "marie",
		Type:     userpb.UserType_USER_TYPE_PRIMARY,
	},
	Username: "marie",
	Groups:   []string{"radium-lovers", "cernbox-projects-eos-readers", "physics-lovers"},
}

var lightweight = &userpb.User{
	Id: &userpb.UserId{
		Idp:      "something-external.org",
		OpaqueId: "0123456789",
		Type:     userpb.UserType_USER_TYPE_LIGHTWEIGHT,
	},
	Username: "0123456789",
	Groups:   []string{"radium-lovers", "cernbox-projects-eos-readers"},
}

var projectDescription1 = memory.SpaceDescription{
	ID:      "cernbox",
	Path:    "/eos/project/c/cernbox",
	Name:    "cernbox",
	Owner:   "cboxsvc",
	Readers: "cernbox-projects-cernbox-readers",
	Writers: "cernbox-projects-cernbox-writers",
	Admins:  "cernbox-projects-cernbox-admins",
}
var projectDescription2 = memory.SpaceDescription{
	ID:      "eos",
	Path:    "/eos/project/e/eos",
	Name:    "eos",
	Owner:   "eossvc",
	Readers: "cernbox-projects-eos-readers",
	Writers: "cernbox-projects-eos-writers",
	Admins:  "cernbox-projects-eos-admins",
}

var projectSpace1 = &provider.StorageSpace{
	Id:        &provider.StorageSpaceId{OpaqueId: projectDescription1.ID},
	Owner:     &userpb.User{Id: &userpb.UserId{OpaqueId: projectDescription1.Owner}},
	Name:      projectDescription1.Name,
	SpaceType: spaces.SpaceTypeProject.AsString(),
	RootInfo:  &provider.ResourceInfo{Path: projectDescription1.Path},
}
var projectSpace2 = &provider.StorageSpace{
	Id:        &provider.StorageSpaceId{OpaqueId: projectDescription2.ID},
	Owner:     &userpb.User{Id: &userpb.UserId{OpaqueId: projectDescription2.Owner}},
	Name:      projectDescription2.Name,
	SpaceType: spaces.SpaceTypeProject.AsString(),
	RootInfo:  &provider.ResourceInfo{Path: projectDescription2.Path},
}

func TestListSpaces(t *testing.T) {
	tests := []struct {
		config   *memory.Config
		user     *userpb.User
		expected []*provider.StorageSpace
	}{
		{
			config: &memory.Config{
				UserSpace: "/home",
			},
			user: einstein,
			expected: []*provider.StorageSpace{
				{
					Id:        &provider.StorageSpaceId{OpaqueId: "/home"},
					Owner:     einstein,
					Name:      einstein.Username,
					SpaceType: spaces.SpaceTypeHome.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path: "/home",
					},
				},
			},
		},
		{
			config: &memory.Config{
				UserSpace: "/home",
			},
			user:     lightweight,
			expected: []*provider.StorageSpace{},
		},
		{
			config: &memory.Config{
				UserSpace: "/home/{{ .Username }}",
				Spaces: []memory.SpaceDescription{
					projectDescription1,
					projectDescription2,
				},
			},
			user: einstein,
			expected: []*provider.StorageSpace{
				{
					Id:        &provider.StorageSpaceId{OpaqueId: "/home/einstein"},
					Owner:     einstein,
					Name:      einstein.Username,
					SpaceType: spaces.SpaceTypeHome.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path: "/home/einstein",
					},
				},
				projectSpace1,
			},
		},
		{
			config: &memory.Config{
				UserSpace: "/home/{{ .Username }}",
				Spaces: []memory.SpaceDescription{
					projectDescription1,
					projectDescription2,
				},
			},
			user: marie,
			expected: []*provider.StorageSpace{
				{
					Id:        &provider.StorageSpaceId{OpaqueId: "/home/marie"},
					Owner:     marie,
					Name:      marie.Username,
					SpaceType: spaces.SpaceTypeHome.AsString(),
					RootInfo: &provider.ResourceInfo{
						Path: "/home/marie",
					},
				},
				projectSpace2,
			},
		},
		{
			config: &memory.Config{
				UserSpace: "/home",
				Spaces: []memory.SpaceDescription{
					projectDescription1,
					projectDescription2,
				},
			},
			user: lightweight,
			expected: []*provider.StorageSpace{
				projectSpace2,
			},
		},
	}

	for _, tt := range tests {
		s, err := memory.NewWithConfig(context.Background(), tt.config)
		if err != nil {
			t.Fatalf("got unexpected error creating new memory spaces provider: %+v", err)
		}

		got, err := s.ListSpaces(context.Background(), tt.user, nil)
		if err != nil {
			t.Fatalf("got unexpected error getting list of spaces: %+v", err)
		}

		if !slices.EqualFunc(tt.expected, got, func(s1, s2 *provider.StorageSpace) bool {
			return s1.Id != nil && s2.Id != nil && s1.Id.OpaqueId == s2.Id.OpaqueId &&
				s1.Name == s2.Name &&
				s1.SpaceType == s2.SpaceType &&
				utils.UserEqual(s1.Owner.Id, s2.Owner.Id) &&
				s1.RootInfo.Path == s2.RootInfo.Path
		}) {
			t.Fatalf("got different result. expected=%+v got=%+v", tt.expected, got)
		}
	}
}
