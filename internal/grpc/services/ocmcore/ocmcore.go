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

package ocmcore

// This package implements the core OCM API for receiving external shares from remote EFSS systems.

import (
	"context"
	"fmt"
	"time"

	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/cs3org/reva/v3/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/v3/pkg/plugin"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rgrpc/status"
	"github.com/cs3org/reva/v3/pkg/utils"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmcore", New)
	plugin.RegisterNamespace("grpc.services.ocmcore.drivers", func(name string, newFunc any) {
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
	repo share.Repository
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	ocmcore.RegisterOcmCoreAPIServer(ss, s)
}

func getShareRepository(ctx context.Context, c *config) (share.Repository, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound(fmt.Sprintf("driver not found: %s", c.Driver))
}

// New creates a new ocm core svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	repo, err := getShareRepository(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: &c,
		repo: repo,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.ocm.core.v1beta1.OcmCoreAPI/CreateOCMCoreShare"}
}

// CreateOCMCoreShare is called when a remote OCM request comes into this reva instance.
func (s *service) CreateOCMCoreShare(ctx context.Context, req *ocmcore.CreateOCMCoreShareRequest) (*ocmcore.CreateOCMCoreShareResponse, error) {
	if req.ShareType != ocm.ShareType_SHARE_TYPE_USER {
		return nil, errtypes.NotSupported("share type not supported")
	}

	now := &typesv1beta1.Timestamp{
		Seconds: uint64(time.Now().Unix()),
	}

	share, err := s.repo.StoreReceivedShare(ctx, &ocm.ReceivedShare{
		RemoteShareId: req.ResourceId,
		Name:          req.Name,
		Grantee: &providerpb.Grantee{
			Type: providerpb.GranteeType_GRANTEE_TYPE_USER,
			Id: &providerpb.Grantee_UserId{
				UserId: req.ShareWith,
			},
		},
		ResourceType: req.ResourceType,
		ShareType:    req.ShareType,
		Owner:        req.Owner,
		Creator:      req.Sender,
		Protocols:    req.Protocols,
		Ctime:        now,
		Mtime:        now,
		Expiration:   req.Expiration,
		State:        ocm.ShareState_SHARE_STATE_PENDING,
	})
	if err != nil {
		// TODO: identify errors
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	return &ocmcore.CreateOCMCoreShareResponse{
		Status:  status.NewOK(ctx),
		Id:      share.Id.OpaqueId,
		Created: share.Ctime,
	}, nil
}

func (s *service) UpdateOCMCoreShare(ctx context.Context, req *ocmcore.UpdateOCMCoreShareRequest) (*ocmcore.UpdateOCMCoreShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}

func (s *service) DeleteOCMCoreShare(ctx context.Context, req *ocmcore.DeleteOCMCoreShareRequest) (*ocmcore.DeleteOCMCoreShareResponse, error) {
	return nil, errtypes.NotSupported("not implemented")
}
