// Copyright 2018-2026 CERN
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
	"fmt"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/utils"
)

type spaceByID interface {
	SpaceByID(ctx context.Context, spaceID string) (*provider.StorageSpace, error)
}

type spaceArgs struct {
	ID string `arg:"id,required" help:"the space id"`
}

// Invocations implements invoke.Invokable.
func (s *service) Invocations() []invoke.InvocationSpec { return s.set.Invocations() }

// Invoke implements invoke.Invokable.
func (s *service) Invoke(ctx context.Context, name string, args map[string]any) (invoke.Result, error) {
	return s.set.Invoke(ctx, name, args)
}

func (s *service) invocations() *invoke.Set {
	set := invoke.NewSet()
	set.Add("space", "Show a project space and the users and groups of its roles, whoever the caller is.").
		Handle(invoke.Typed(s.space))
	return set
}

func (s *service) space(ctx context.Context, a spaceArgs) (invoke.Result, error) {
	c, ok := s.projects.(spaceByID)
	if !ok {
		return nil, fmt.Errorf("the %s driver cannot look up a space by id", s.c.Driver)
	}
	space, err := c.SpaceByID(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	roles := make([]map[string]any, 0, len(space.GetRoles()))
	for _, r := range space.GetRoles() {
		users, groups := []string{}, []string{}
		for _, g := range r.GetRecipients() {
			userID, groupID := utils.ExtractGranteeID(g)
			if userID != nil {
				users = append(users, userID.GetOpaqueId())
			}
			if groupID != nil {
				groups = append(groups, groupID.GetOpaqueId())
			}
		}
		roles = append(roles, map[string]any{"name": r.GetRoleName(), "users": users, "groups": groups})
	}
	return invoke.Result{"id": space.GetId().GetOpaqueId(), "name": space.GetName(), "type": space.GetSpaceType(), "roles": roles}, nil
}
