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

package gateway

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/storage/utils/templates"
)

// ensureLightweightHome makes sure the lightweight account u can reach its
// home at lightweight_home_layout. If not, it calls CreateHome on the storage
// provider holding that path, whose hook creates the folder and shares it.
func (s *svc) ensureLightweightHome(ctx context.Context, u *userpb.User) error {
	if _, err := s.createHomeCache.Get(u.Id.OpaqueId); err == nil {
		return nil
	}

	home := templates.WithUser(u, s.c.LightweightHomeLayout)

	// A lightweight account can stat any path, but only gets permissions
	// on the ones shared with it.
	statRes, err := s.Stat(ctx, &provider.StatRequest{Ref: &provider.Reference{Path: home}})
	if err != nil {
		return errors.Wrap(err, "error statting lightweight home")
	}
	switch {
	case statRes.Status.Code == rpc.Code_CODE_OK && statRes.Info.GetPermissionSet().GetStat():
		s.markHomeCreated(u)
		return nil
	case statRes.Status.Code == rpc.Code_CODE_OK, statRes.Status.Code == rpc.Code_CODE_NOT_FOUND:
	default:
		return status.NewErrorFromCode(statRes.Status.Code, "gateway")
	}

	c, err := s.findByPath(ctx, home)
	if err != nil {
		return errors.Wrap(err, "error finding storage provider for lightweight home")
	}
	createRes, err := c.CreateHome(ctx, &provider.CreateHomeRequest{})
	if err != nil {
		return errors.Wrap(err, "error calling CreateHome")
	}
	if createRes.Status.Code != rpc.Code_CODE_OK {
		return status.NewErrorFromCode(createRes.Status.Code, "gateway")
	}

	s.markHomeCreated(u)
	return nil
}

func (s *svc) markHomeCreated(u *userpb.User) {
	if s.c.CreateHomeCacheTTL > 0 {
		_ = s.createHomeCache.Set(u.Id.OpaqueId, true)
	}
}
