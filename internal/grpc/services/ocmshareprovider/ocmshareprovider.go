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
	"net/url"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	providerpb "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typespb "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/internal/http/services/ocmd"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/client"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
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
	ProviderDomain string                            `mapstructure:"provider_domain" docs:"The same domain registered in the provider authorizer"`
	WebappTemplate string                            `mapstructure:"webapp_template"`
}

type service struct {
	conf       *config
	repo       share.Repository
	client     *client.OCMClient
	gateway    gateway.GatewayAPIClient
	webappTmpl *template.Template
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
	if c.ClientTimeout == 0 {
		c.ClientTimeout = 10
	}
	if c.WebappTemplate == "" {
		c.WebappTemplate = "https://cernbox.cern.ch/external/sciencemesh/{{.Token}}{relative-path-to-shared-resource}"
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

	tpl, err := template.New("webapp_template").Parse(c.WebappTemplate)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:       c,
		repo:       repo,
		client:     client,
		gateway:    gateway,
		webappTmpl: tpl,
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

func getResourceType(info *providerpb.ResourceInfo) string {
	switch info.Type {
	case providerpb.ResourceType_RESOURCE_TYPE_FILE:
		return "file"
	case providerpb.ResourceType_RESOURCE_TYPE_CONTAINER:
		return "folder"
	}
	return "unknown"
}

func (s *service) webdavURL(ctx context.Context, path string) string {
	// the url is in the form of https://cernbox.cern.ch/remote.php/dav/files/gdelmont/eos/user/g/gdelmont
	user := ctxpkg.ContextMustGetUser(ctx)
	p, err := url.JoinPath(s.conf.WebDAVPrefix, user.Username, path)
	if err != nil {
		panic(err)
	}
	return p
}

func (s *service) getWebdavProtocol(ctx context.Context, info *providerpb.ResourceInfo, m *ocm.AccessMethod_WebdavOptions) *ocmd.WebDAV {
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

func (s *service) getWebappProtocol(share *ocm.Share) *ocmd.Webapp {
	var b strings.Builder
	if err := s.webappTmpl.Execute(&b, share); err != nil {
		panic(err)
	}
	return &ocmd.Webapp{
		URITemplate: b.String(),
	}
}

func (s *service) getProtocols(ctx context.Context, share *ocm.Share, info *providerpb.ResourceInfo, methods []*ocm.AccessMethod) ocmd.Protocols {
	var p ocmd.Protocols
	for _, m := range methods {
		switch t := m.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			p = append(p, s.getWebdavProtocol(ctx, info, t))
		case *ocm.AccessMethod_WebappOptions:
			p = append(p, s.getWebappProtocol(share))
		case *ocm.AccessMethod_TransferOptions:
			// TODO
		}
	}
	return p
}

func (s *service) CreateOCMShare(ctx context.Context, req *ocm.CreateOCMShareRequest) (*ocm.CreateOCMShareResponse, error) {
	statRes, err := s.gateway.Stat(ctx, &providerpb.StatRequest{
		Ref: &providerpb.Reference{
			ResourceId: req.ResourceId,
		},
	})
	if err != nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, err
	}

	if statRes.Status.Code != rpc.Code_CODE_OK {
		if statRes.Status.Code == rpc.Code_CODE_NOT_FOUND {
			return &ocm.CreateOCMShareResponse{
				Status: status.NewNotFound(ctx, statRes.Status.Message),
			}, nil
		}
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, errors.New(statRes.Status.Message), statRes.Status.Message),
		}, nil
	}

	info := statRes.Info
	user := ctxpkg.ContextMustGetUser(ctx)
	tkn := utils.RandString(32)
	now := time.Now().UnixNano()
	ts := &typespb.Timestamp{
		Seconds: uint64(now / 1000000000),
		Nanos:   uint32(now % 1000000000),
	}

	ocmshare := &ocm.Share{
		Token:         tkn,
		Name:          filepath.Base(info.Path),
		ResourceId:    req.ResourceId,
		Grantee:       req.Grantee,
		ShareType:     ocm.ShareType_SHARE_TYPE_USER,
		Owner:         info.Owner,
		Creator:       user.Id,
		Ctime:         ts,
		Mtime:         ts,
		Expiration:    req.Expiration,
		AccessMethods: req.AccessMethods,
	}

	ocmshare, err = s.repo.StoreShare(ctx, ocmshare)
	if err != nil {
		if errors.Is(err, share.ErrShareAlreadyExisting) {
			return &ocm.CreateOCMShareResponse{
				Status: status.NewAlreadyExists(ctx, err, "share already exists"),
			}, nil
		}
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInternal(ctx, err, err.Error()),
		}, nil
	}

	ocmEndpoint, err := getOCMEndpoint(req.RecipientMeshProvider)
	if err != nil {
		return &ocm.CreateOCMShareResponse{
			Status: status.NewInvalidArg(ctx, "the selected provider does not have an OCM endpoint"),
		}, nil
	}

	newShareReq := &client.NewShareRequest{
		ShareWith:  formatOCMUser(req.Grantee.GetUserId()),
		Name:       ocmshare.Name,
		ResourceID: fmt.Sprintf("%s:%s", req.ResourceId.StorageId, req.ResourceId.OpaqueId),
		Owner: formatOCMUser(&userpb.UserId{
			OpaqueId: info.Owner.OpaqueId,
			Idp:      s.conf.ProviderDomain, // FIXME: this is not generally true in case of resharing
		}),
		Sender: formatOCMUser(&userpb.UserId{
			OpaqueId: user.Id.OpaqueId,
			Idp:      s.conf.ProviderDomain,
		}),
		SenderDisplayName: user.DisplayName,
		ShareType:         "user",
		ResourceType:      getResourceType(info),
		Protocols:         s.getProtocols(ctx, ocmshare, info, req.AccessMethods),
	}

	if req.Expiration != nil {
		newShareReq.Expiration = req.Expiration.Seconds
	}

	newShareRes, err := s.client.NewShare(ctx, ocmEndpoint, newShareReq)
	if err != nil {
		switch {
		case errors.Is(err, client.ErrInvalidParameters):
			return &ocm.CreateOCMShareResponse{
				Status: status.NewInvalidArg(ctx, err.Error()),
			}, nil
		case errors.Is(err, client.ErrServiceNotTrusted):
			return &ocm.CreateOCMShareResponse{
				Status: status.NewInvalidArg(ctx, err.Error()),
			}, nil
		default:
			return &ocm.CreateOCMShareResponse{
				Status: status.NewInternal(ctx, err, err.Error()),
			}, nil
		}
	}

	res := &ocm.CreateOCMShareResponse{
		Status:               status.NewOK(ctx),
		Share:                ocmshare,
		RecipientDisplayName: newShareRes.RecipientDisplayName,
	}
	return res, nil
}

func (s *service) RemoveOCMShare(ctx context.Context, req *ocm.RemoveOCMShareRequest) (*ocm.RemoveOCMShareResponse, error) {
	// TODO (gdelmont): notify the remote provider using the /notification ocm endpoint
	// https://cs3org.github.io/OCM-API/docs.html?branch=develop&repo=OCM-API&user=cs3org#/paths/~1notifications/post
	user := ctxpkg.ContextMustGetUser(ctx)
	if err := s.repo.DeleteShare(ctx, user, req.Ref); err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.RemoveOCMShareResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
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
	ocmshare, err := s.repo.GetShare(ctx, user, req.Ref)
	if err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.GetOCMShareResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
		return &ocm.GetOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting share"),
		}, nil
	}

	return &ocm.GetOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  ocmshare,
	}, nil
}

func (s *service) ListOCMShares(ctx context.Context, req *ocm.ListOCMSharesRequest) (*ocm.ListOCMSharesResponse, error) {
	user := ctxpkg.ContextMustGetUser(ctx)
	shares, err := s.repo.ListShares(ctx, user, req.Filters)
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
	_, err := s.repo.UpdateShare(ctx, user, req.Ref, req.Field.GetPermissions()) // TODO(labkode): check what to update
	if err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.UpdateOCMShareResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
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
	shares, err := s.repo.ListReceivedShares(ctx, user)
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
	user := ctxpkg.ContextMustGetUser(ctx)
	_, err := s.repo.UpdateReceivedShare(ctx, user, req.Share, req.UpdateMask) // TODO(labkode): check what to update
	if err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.UpdateReceivedOCMShareResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
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
	ocmshare, err := s.repo.GetReceivedShare(ctx, user, req.Ref)
	if err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.GetReceivedOCMShareResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
		return &ocm.GetReceivedOCMShareResponse{
			Status: status.NewInternal(ctx, err, "error getting received share"),
		}, nil
	}

	res := &ocm.GetReceivedOCMShareResponse{
		Status: status.NewOK(ctx),
		Share:  ocmshare,
	}
	return res, nil
}
