// Copyright 2018-2021 CERN
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
	"encoding/json"
	"fmt"
	"strings"

	ocmcore "github.com/cs3org/go-cs3apis/cs3/ocm/core/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmcore", New)
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

type service struct {
	conf *config
	sm   share.Manager
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	ocmcore.RegisterOcmCoreAPIServer(ss, s)
}

func getShareManager(c *config) (share.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new ocm core svc
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	sm, err := getShareManager(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		sm:   sm,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.ocm.core.v1beta1.OcmCoreAPI/CreateOCMCoreShare"}
}

func (s *service) CreateOCMCoreShare(ctx context.Context, req *ocmcore.CreateOCMCoreShareRequest) (*ocmcore.CreateOCMCoreShareResponse, error) {
	parts := strings.Split(req.ProviderId, ":")
	if len(parts) < 2 {
		err := errors.New("resource ID does not follow the layout storageid:opaqueid " + req.ProviderId)
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "error decoding resource ID"),
		}, nil
	}

	resource := &provider.ResourceId{
		StorageId: parts[0],
		OpaqueId:  parts[1],
	}

	var resourcePermissions *provider.ResourcePermissions
	permOpaque, ok := req.Protocol.Opaque.Map["permissions"]
	if !ok {
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, errors.New("resource permissions not set"), ""),
		}, nil
	}
	switch permOpaque.Decoder {
	case "json":
		err := json.Unmarshal(permOpaque.Value, &resourcePermissions)
		if err != nil {
			return &ocmcore.CreateOCMCoreShareResponse{
				Status: status.NewInternal(ctx, err, "error decoding resource permissions"),
			}, nil
		}
	default:
		err := errors.New("opaque entry decoder not recognized")
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "invalid opaque entry decoder"),
		}, nil
	}

	var token string
	tokenOpaque, ok := req.Protocol.Opaque.Map["token"]
	if !ok {
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, errors.New("token not set"), ""),
		}, nil
	}
	switch tokenOpaque.Decoder {
	case "plain":
		token = string(tokenOpaque.Value)
	default:
		err := errors.New("opaque entry decoder not recognized: " + tokenOpaque.Decoder)
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "invalid opaque entry decoder"),
		}, nil
	}

	grant := &ocm.ShareGrant{
		Grantee: &provider.Grantee{
			Type: provider.GranteeType_GRANTEE_TYPE_USER,
			// For now, we only support user shares.
			// TODO (ishank011): To be updated once this is decided.
			Id: &provider.Grantee_UserId{UserId: req.ShareWith},
		},
		Permissions: &ocm.SharePermissions{
			Permissions: resourcePermissions,
		},
	}

	share, err := s.sm.Share(ctx, resource, grant, req.Name, nil, "", req.Owner, token, ocm.Share_SHARE_TYPE_REGULAR)
	if err != nil {
		return &ocmcore.CreateOCMCoreShareResponse{
			Status: status.NewInternal(ctx, err, "error creating ocm core share"),
		}, nil
	}

	res := &ocmcore.CreateOCMCoreShareResponse{
		Status:  status.NewOK(ctx),
		Id:      share.Id.OpaqueId,
		Created: share.Ctime,
	}
	return res, nil
}
