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

package ocmshareprovider

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/ocmd"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/client"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/manager/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmshareprovider", New)
}

type config struct {
	Driver         string                            `mapstructure:"driver"`
	Drivers        map[string]map[string]interface{} `mapstructure:"drivers"`
	ClientTimeout  int                               `mapstructure:"client_timeout"`
	ClientInsecure bool                              `mapstructure:"client_insecure"`
	GatewaySVC     string                            `mapstructure:"gatewaysvc"`
	WebDAVPrefix   string                            `mapstructure:"webdav_prefix"`
}

type service struct {
	conf    *config
	repo    share.Repository
	client  *client.OCMClient
	gateway gateway.GatewayAPIClient
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
	if c.ClientTimeout == 0 {
		c.ClientTimeout = 10
	}

	c.GatewaySVC = sharedconf.GetGatewaySVC(c.GatewaySVC)
}

func (s *service) Register(ss *grpc.Server) {
	ocm.RegisterOcmAPIServer(ss, s)
}

func getShareRepository(c *config) (share.Repository, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New creates a new ocm share provider svc.
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}
	c.init()

	repo, err := getShareRepository(c)
	if err != nil {
		return nil, err
	}

	client := client.New(&client.Config{
		Timeout:  time.Duration(c.ClientTimeout) * time.Second,
		Insecure: c.ClientInsecure,
	})

	gateway, err := pool.GetGatewayServiceClient(pool.Endpoint(c.GatewaySVC))
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:    c,
		repo:    repo,
		client:  client,
		gateway: gateway,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return nil
}

func getOCMEndpoint(originProvider *ocmprovider.ProviderInfo) (string, error) {
	for _, s := range originProvider.Services {
		if s.Endpoint.Type.Name == "OCM" {
			return s.Endpoint.Path, nil
		}
	}
	return "", errors.New("ocm endpoint not specified for mesh provider")
}

func formatOCMUser(u *userpb.UserId) string {
	return fmt.Sprintf("%s@%s", u.OpaqueId, u.Idp)
}

func getResourceType(info *providerv1beta1.ResourceInfo) string {
	switch info.Type {
	case providerv1beta1.ResourceType_RESOURCE_TYPE_FILE:
		return "file"
	case providerv1beta1.ResourceType_RESOURCE_TYPE_CONTAINER:
		return "folder"
	}
	return "unknown"
}

func (s *service) webdavURL(ctx context.Context, path string) string {
	// the url is in the form of https://cernbox.cern.ch/remote.php/dav/files/gdelmont/eos/user/g/gdelmont
	user := ctxpkg.ContextMustGetUser(ctx)
	return filepath.Join(s.conf.WebDAVPrefix, user.Username, path)
}

func (s *service) getWebdavProtocol(ctx context.Context, info *providerv1beta1.ResourceInfo, m *ocm.AccessMethod_WebdavOptions) *ocmd.WebDAV {
	var perms []string
	if m.WebdavOptions.Permissions.InitiateFileDownload {
		perms = append(perms, "read")
	}
	if m.WebdavOptions.Permissions.InitiateFileUpload {
		perms = append(perms, "write")
	}

	return &ocmd.WebDAV{
		SharedSecret: ctxpkg.ContextMustGetToken(ctx), // TODO: change this and use an ocm token
		Permissions:  perms,
		URL:          s.webdavURL(ctx, info.Path), // TODO: change this and use an endpoint for ocm
	}
}

func (s *service) getProtocols(ctx context.Context, info *providerv1beta1.ResourceInfo, methods []*ocm.AccessMethod) ocmd.Protocols {
	var p ocmd.Protocols
	for _, m := range methods {
		switch t := m.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			p = append(p, s.getWebdavProtocol(ctx, info, t))
		case *ocm.AccessMethod_WebappOptions:
			// TODO
		case *ocm.AccessMethod_DatatxOptions:
			// TODO
		}
	}
	return p
}

func (s *service) CreateOCMShare(ctx context.Context, req *ocm.CreateOCMShareRequest) (*ocm.CreateOCMShareResponse, error) {
	statRes, err := s.gateway.Stat(ctx, &providerv1beta1.StatRequest{
		Ref: &providerv1beta1.Reference{
			ResourceId: req.ResourceId,
		},
	})
	if err != nil {
		return nil, err
	}

	if statRes.Status.Code != rpcv1beta1.Code_CODE_OK {
		// TODO: review error codes
		return nil, errtypes.InternalError(statRes.Status.Message)
	}

	info := statRes.Info
	user := ctxpkg.ContextMustGetUser(ctx)

	share := &ocm.Share{
		Name:          filepath.Base(info.Path),
		ResourceId:    req.ResourceId,
		Grantee:       req.Grantee,
		Owner:         info.Owner,
		Creator:       user.Id,
		AccessMethods: req.AccessMethods,
	}

	share, err = s.repo.StoreShare(ctx, share)
	if err != nil {
		// TODO: err
		return nil, errtypes.InternalError(err.Error())
	}

	ocmEndpoint, err := getOCMEndpoint(req.RecipientMeshProvider)
	if err != nil {
		// TODO: err
		return nil, errtypes.InternalError(err.Error())
	}

	err = s.client.NewShare(ctx, ocmEndpoint, &client.NewShareRequest{
		ShareWith:         req.Grantee.GetGroupId().OpaqueId,
		Name:              share.Name,
		ResourceID:        fmt.Sprintf("%s:%s", req.ResourceId.StorageId, req.ResourceId.OpaqueId),
		Owner:             formatOCMUser(info.Owner),
		Sender:            formatOCMUser(user.Id),
		SenderDisplayName: user.DisplayName,
		ShareType:         "user",
		ResourceType:      getResourceType(info),
		Protocols:         s.getProtocols(ctx, info, req.AccessMethods),
	})

	if err != nil {
		// TODO: err
		return nil, errtypes.InternalError(err.Error())
	}

	res := &ocm.CreateOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  share,
	}
	return res, nil
}

func (s *service) RemoveOCMShare(ctx context.Context, req *ocm.RemoveOCMShareRequest) (*ocm.RemoveOCMShareResponse, error) {
	// TODO (gdelmont): notify the remote provider using the /notification ocm endpoint
	// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
	user := ctxpkg.ContextMustGetUser(ctx)
	if err := s.repo.DeleteShare(ctx, user.Id, req.Ref); err != nil {
		// TODO: error
		return &ocm.RemoveOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error removing share"),
		}, nil
	}

	return &ocm.RemoveOCMShareResponse{
		Status: status.NewOK(ctx),
	}, nil
}

func (s *service) GetOCMShare(ctx context.Context, req *ocm.GetOCMShareRequest) (*ocm.GetOCMShareResponse, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	share, err := s.repo.GetShare(ctx, user.Id, req.Ref)
	if err != nil {
		// TODO: error
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
	user := ctxpkg.ContextMustGetUser(ctx)
	shares, err := s.repo.ListShares(ctx, user.Id, req.Filters)
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
	user := ctxpkg.ContextMustGetUser(ctx)
	_, err := s.repo.UpdateShare(ctx, user.Id, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		// TODO: error
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
	user := ctxpkg.ContextMustGetUser(ctx)
	shares, err := s.repo.ListReceivedShares(ctx, user.Id)
	if err != nil {
		// TODO: error
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
	user := ctxpkg.ContextMustGetUser(ctx)
	_, err := s.repo.UpdateReceivedShare(ctx, user.Id, req.Share, req.UpdateMask) // TODO(labkode): check what to update
	if err != nil {
		// TODO: error
		return &ocm.UpdateReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error updating received share"),
		}, nil
	}

	res := &ocm.UpdateReceivedOCMShareResponse{
		Status: status.NewOK(ctx),
	}
	return res, nil
}

func (s *service) GetReceivedOCMShare(ctx context.Context, req *ocm.GetReceivedOCMShareRequest) (*ocm.GetReceivedOCMShareResponse, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	share, err := s.repo.GetReceivedShare(ctx, user.Id, req.Ref)
	if err != nil {
		// TODO: error
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
