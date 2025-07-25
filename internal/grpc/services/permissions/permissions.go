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

package permissions

import (
	"context"
	"fmt"

	permissions "github.com/cs3org/go-cs3apis/cs3/permissions/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permission"
	"github.com/cs3org/reva/v3/pkg/permission/manager/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("permissions", New)
	plugin.RegisterNamespace("grpc.services.permissions.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                            `docs:"localhome;The permission driver to be used." mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `docs:"url:pkg/permission/permission.go"            mapstructure:"drivers"`
}

type service struct {
	manager permission.Manager
}

// New returns a new PermissionsServiceServer.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	f, ok := registry.NewFuncs[c.Driver]
	if !ok {
		return nil, fmt.Errorf("could not get permission manager '%s'", c.Driver)
	}
	manager, err := f(ctx, c.Drivers[c.Driver])
	if err != nil {
		return nil, err
	}

	service := &service{manager: manager}
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	permissions.RegisterPermissionsAPIServer(ss, s)
}

func (s *service) CheckPermission(ctx context.Context, req *permissions.CheckPermissionRequest) (*permissions.CheckPermissionResponse, error) {
	var subject string
	switch ref := req.SubjectRef.Spec.(type) {
	case *permissions.SubjectReference_UserId:
		subject = ref.UserId.OpaqueId
	case *permissions.SubjectReference_GroupId:
		subject = ref.GroupId.OpaqueId
	}
	var status *rpc.Status
	if ok := s.manager.CheckPermission(req.Permission, subject, req.Ref); ok {
		status = &rpc.Status{Code: rpc.Code_CODE_OK}
	} else {
		status = &rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED}
	}
	return &permissions.CheckPermissionResponse{Status: status}, nil
}
