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

package ocmshareprovider

import (
	"context"
	"path"
	"strings"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/datatx"
	datatxreg "github.com/cs3org/reva/pkg/datatx/manager/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/token"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmshareprovider", New)
}

type config struct {
	Driver        string                            `mapstructure:"driver"`
	Drivers       map[string]map[string]interface{} `mapstructure:"drivers"`
	DatatxDriver  string                            `mapstructure:"datatxdriver"`
	DatatxDrivers map[string]map[string]interface{} `mapstructure:"datatxdrivers"`
}

type service struct {
	conf *config
	sm   share.Manager
	dtxm datatx.Manager
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func (s *service) Register(ss *grpc.Server) {
	ocm.RegisterOcmAPIServer(ss, s)
}

func getShareManager(c *config) (share.Manager, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func getDatatxManager(c *config) (datatx.Manager, error) {
	if f, ok := datatxreg.NewFuncs[c.DatatxDriver]; ok {
		return f(c.DatatxDrivers[c.DatatxDriver])
	}
	return nil, errtypes.NotFound("datatx driver not found: " + c.DatatxDriver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new ocm share provider svc
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

	dtxm, err := getDatatxManager(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf: c,
		sm:   sm,
		dtxm: dtxm,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) CreateOCMShare(ctx context.Context, req *ocm.CreateOCMShareRequest) (*ocm.CreateOCMShareResponse, error) {

	if req.Opaque == nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, errtypes.BadRequest("can't find resource permissions"), ""),
		}, nil
	}

	var permissions string
	permOpaque, ok := req.Opaque.Map["permissions"]
	if !ok {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, errtypes.BadRequest("resource permissions not set"), ""),
		}, nil
	}
	switch permOpaque.Decoder {
	case "plain":
		permissions = string(permOpaque.Value)
	default:
		err := errtypes.NotSupported("opaque entry decoder not recognized: " + permOpaque.Decoder)
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "invalid opaque entry decoder"),
		}, nil
	}

	var name string
	nameOpaque, ok := req.Opaque.Map["name"]
	if !ok {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, errtypes.BadRequest("resource name not set"), ""),
		}, nil
	}
	switch nameOpaque.Decoder {
	case "plain":
		name = string(nameOpaque.Value)
	default:
		err := errtypes.NotSupported("opaque entry decoder not recognized: " + nameOpaque.Decoder)
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "invalid opaque entry decoder"),
		}, nil
	}

	// discover share type
	sharetype := ocm.Share_SHARE_TYPE_REGULAR
	protocol, ok := req.Opaque.Map["protocol"]
	if ok {
		switch protocol.Decoder {
		case "plain":
			if string(protocol.Value) == "datatx" {
				sharetype = ocm.Share_SHARE_TYPE_TRANSFER
			}
		default:
			err := errors.New("protocol decoder not recognized")
			return &ocm.CreateOCMShareResponse{
				Status: status.NewInternal(ctx, err, "error creating share"),
			}, nil
		}
	}

	share, err := s.sm.Share(ctx, req.ResourceId, req.Grant, name, req.RecipientMeshProvider, permissions, nil, "", sharetype)
	if err != nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error creating share"),
		}, nil
	}

	res := &ocm.CreateOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemoveOCMShare(ctx context.Context, req *ocm.RemoveOCMShareRequest) (*ocm.RemoveOCMShareResponse, error) {
	err := s.sm.Unshare(ctx, req.Ref)
	if err != nil {
		return &ocm.RemoveOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error removing share"),
		}, nil
	}

	return &ocm.RemoveOCMShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	share, err := s.sm.GetShare(ctx, req.Ref)
	if err != nil {
		return &ocm.GetOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share"),
		}, nil
	}

	return &ocm.GetOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}, nil
}

func (s *service) ListOCMShares(ctx context.Context, req *ocm.ListOCMSharesRequest) (*ocm.ListOCMSharesResponse, error) {
	shares, err := s.sm.ListShares(ctx, req.Filters) // TODO(labkode): add filter to share manager
	if err != nil {
		return &ocm.ListOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing shares"),
		}, nil
	}

	res := &ocm.ListOCMSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateOCMShare(ctx context.Context, req *ocm.UpdateOCMShareRequest) (*ocm.UpdateOCMShareResponse, error) {
	_, err := s.sm.UpdateShare(ctx, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		return &ocm.UpdateOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error updating share"),
		}, nil
	}

	res := &ocm.UpdateOCMShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) ListReceivedOCMShares(ctx context.Context, req *ocm.ListReceivedOCMSharesRequest) (*ocm.ListReceivedOCMSharesResponse, error) {
	shares, err := s.sm.ListReceivedShares(ctx)
	if err != nil {
		return &ocm.ListReceivedOCMSharesResponse{
			Status: status.NewInternal(ctx, err, "error listing received shares"),
		}, nil
	}

	res := &ocm.ListReceivedOCMSharesResponse{
		Status: status.NewOK(ctx),
		Shares: shares,
	}
	return res, nil
}

func (s *service) UpdateReceivedOCMShare(ctx context.Context, req *ocm.UpdateReceivedOCMShareRequest) (*ocm.UpdateReceivedOCMShareResponse, error) {
	log := appctx.GetLogger(ctx)

	_, err := s.sm.UpdateReceivedShare(ctx, req.Ref, req.Field) // TODO(labkode): check what to update
	if err != nil {
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	// initiate transfer in case this is a transfer type share
	receivedShare, err := s.sm.GetReceivedShare(ctx, req.Ref)
	if err != nil {
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}
	if receivedShare.GetShare().ShareType == ocm.Share_SHARE_TYPE_TRANSFER {
		srcRemote := receivedShare.GetShare().GetOwner().GetIdp()
		// remove the home path for webdav transfer calls
		// TODO do we actually know for sure the home path of the src reva instance ??
		srcPath := strings.TrimPrefix(receivedShare.GetShare().GetName(), "/home")
		var srcToken string
		srcTokenOpaque, ok := receivedShare.GetShare().Grantee.Opaque.Map["token"]
		if !ok {
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewNotFound(ctx, "token not found"),
			}, nil
		}
		switch srcTokenOpaque.Decoder {
		case "plain":
			srcToken = string(srcTokenOpaque.Value)
		default:
			err := errtypes.NotSupported("opaque entry decoder not recognized: " + srcTokenOpaque.Decoder)
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewInternal(ctx, err, "error updating received share"),
			}, nil
		}

		destRemote := receivedShare.GetShare().GetGrantee().GetUserId().GetIdp()
		// TODO how to get the data transfers folder?
		destPath := path.Join("/Data-Transfers", path.Base(receivedShare.GetShare().Name))
		destToken, ok := token.ContextGetToken(ctx)
		if !ok || destToken == "" {
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewInternal(ctx, err, "error updating received share"),
			}, nil
		}

		datatxInfoStatus, err := s.dtxm.CreateTransfer(receivedShare.GetShare().GetId().OpaqueId, srcRemote, srcPath, srcToken, destRemote, destPath, destToken)
		if err != nil {
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewInternal(ctx, err, "error updating received share"),
			}, nil
		}
		log.Info().Msg("datatx transfer created: " + datatxInfoStatus.String())

	}

	res := &ocm.UpdateReceivedOCMShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetReceivedOCMShare(ctx context.Context, req *ocm.GetReceivedOCMShareRequest) (*ocm.GetReceivedOCMShareResponse, error) {
	share, err := s.sm.GetReceivedShare(ctx, req.Ref)
	if err != nil {
		return &ocm.GetReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res := &ocm.GetReceivedOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}
