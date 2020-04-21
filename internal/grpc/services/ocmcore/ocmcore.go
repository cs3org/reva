// Copyright 2018-2020 CERN
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

import (
	"context"

	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/sharedconf"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmcore", New)
}

type config struct {
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

type service struct {
	conf *config
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	ocmcore.RegisterOcmCoreAPIServer(ss, s)
}

// New creates a new user ocm core svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c := &config
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	service := &service{
		conf: c,
	}
	return service, nil
}

func (s *service) CreateOCMCoreShare(ctx context.Context, req *ocmcore.CreateOCMShareRequest) (*ocmcore.CreateOCMShareResponse, error) {

	gatewayClient, err := pool.GetGatewayServiceClient(s.c.GatewaySvc)
	if err != nil {
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "error getting grpc client"),
		}, nil
	}

	createShareReq := &ocm.CreateOCMShareRequest{
		ResourceId: &provider.ResourceInfo.Id{
			StorageId: req.ProviderId,
			OpaqueId: req.Name,
		}
		Grant: &ocm.ShareGrant{
			Grantee: &provider.Grantee{
				Type: provider.GranteeType_GRANTEE_TYPE_USER,
				Id:   req.ShareWith,
			},
			Permissions: &ocm.SharePermissions{
				Permissions: req.Protocol.Opaque.Map["permissions"],
			},
		},
	}

	createShareResponse, err := gatewayClient.CreateOCMShare(ctx, createShareReq)
	if err != nil {
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "error creating share"),
		}, nil
	}

	res := &ocmcore.CreateOCMCoreShareResponse{
		Status: status.NewOK(ctx),
		Id: createShareResponse.Share.Id,
		Created:  createShareResponse.Share.Ctime,
	}
	return res, nil
}

func (s *service) GetOCMCoreShare(ctx context.Context, req *ocmcore.GetOCMCoreShareRequest) (*ocmcore.GetOCMCoreShareResponse, error) {
	return &ocmcore.GetOCMCoreShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) ListOCMCoreShares(ctx context.Context, req *ocmcore.ListOCMCoreSharesRequest) (*ocmcore.ListOCMCoreSharesResponse, error) {
	return &ocmcore.ListOCMCoreSharesResponse{
		Status: status.NewOK(ctx),
	}, nil
}
