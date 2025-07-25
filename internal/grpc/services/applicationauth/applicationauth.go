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

package applicationauth

import (
	"context"

	appauthpb "github.com/cs3org/go-cs3apis/cs3/auth/applications/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appauth"
	"github.com/cs3org/reva/v3/pkg/appauth/manager/registry"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("applicationauth", New)
	plugin.RegisterNamespace("grpc.services.applicationauth.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf *config
	am   appauth.Manager
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	appauthpb.RegisterApplicationsAPIServer(ss, s)
}

func getAppAuthManager(ctx context.Context, c *config) (appauth.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// New creates a app auth provider svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	am, err := getAppAuthManager(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: &c,
		am:   am,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.auth.applications.v1beta1.ApplicationsAPI/GetAppPassword"}
}

func (s *service) GenerateAppPassword(ctx context.Context, req *appauthpb.GenerateAppPasswordRequest) (*appauthpb.GenerateAppPasswordResponse, error) {
	pwd, err := s.am.GenerateAppPassword(ctx, req.TokenScope, req.Label, req.Expiration)
	if err != nil {
		return &appauthpb.GenerateAppPasswordResponse{
			Status: status.NewInternal(ctx, err, "error generating app password"),
		}, nil
	}

	return &appauthpb.GenerateAppPasswordResponse{
		Status:      status.NewOK(ctx),
		AppPassword: pwd,
	}, nil
}

func (s *service) ListAppPasswords(ctx context.Context, req *appauthpb.ListAppPasswordsRequest) (*appauthpb.ListAppPasswordsResponse, error) {
	pwds, err := s.am.ListAppPasswords(ctx)
	if err != nil {
		return &appauthpb.ListAppPasswordsResponse{
			Status: status.NewInternal(ctx, err, "error listing app passwords"),
		}, nil
	}

	return &appauthpb.ListAppPasswordsResponse{
		Status:       status.NewOK(ctx),
		AppPasswords: pwds,
	}, nil
}

func (s *service) InvalidateAppPassword(ctx context.Context, req *appauthpb.InvalidateAppPasswordRequest) (*appauthpb.InvalidateAppPasswordResponse, error) {
	err := s.am.InvalidateAppPassword(ctx, req.Password)
	if err != nil {
		return &appauthpb.InvalidateAppPasswordResponse{
			Status: status.NewInternal(ctx, err, "error invalidating app password"),
		}, nil
	}

	return &appauthpb.InvalidateAppPasswordResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetAppPassword(ctx context.Context, req *appauthpb.GetAppPasswordRequest) (*appauthpb.GetAppPasswordResponse, error) {
	pwd, err := s.am.GetAppPassword(ctx, req.User, req.Password)
	if err != nil {
		return &appauthpb.GetAppPasswordResponse{
			Status: status.NewInternal(ctx, err, "error getting app password via username/password"),
		}, nil
	}

	return &appauthpb.GetAppPasswordResponse{
		Status:      status.NewOK(ctx),
		AppPassword: pwd,
	}, nil
}
