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
	"github.com/cs3org/reva/internal/http/services/opencloudmesh/ocmd"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/ocm/client"
	"github.com/cs3org/reva/pkg/ocm/share"
	"github.com/cs3org/reva/pkg/ocm/share/repository/registry"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("ocmshareprovider", New)
	plugin.RegisterNamespace("grpc.services.ocmshareprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type config struct {
	Driver         string                            `mapstructure:"driver"`
	Drivers        map[string]map[string]interface{} `mapstructure:"drivers"`
	ClientTimeout  int                               `mapstructure:"client_timeout"`
	ClientInsecure bool                              `mapstructure:"client_insecure"`
	GatewaySVC     string                            `mapstructure:"gatewaysvc"                                    validate:"required"`
	ProviderDomain string                            `docs:"The same domain registered in the provider authorizer" mapstructure:"provider_domain" validate:"required"`
	WebDAVEndpoint string                            `mapstructure:"webdav_endpoint"                               validate:"required"`
	WebappTemplate string                            `mapstructure:"webapp_template"`
}

type service struct {
	conf       *config
	repo       share.Repository
	client     *client.OCMClient
	gateway    gateway.GatewayAPIClient
	webappTmpl *template.Template
	walker     walker.Walker
}

func (c *config) ApplyDefaults() {
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

func getShareRepository(ctx context.Context, c *config) (share.Repository, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(ctx, c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

// New creates a new ocm share provider svc.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	repo, err := getShareRepository(ctx, &c)
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
	walker := walker.NewWalker(gateway)

	service := &service{
		conf:       &c,
		repo:       repo,
		client:     client,
		gateway:    gateway,
		webappTmpl: tpl,
		walker:     walker,
	}

	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/cs3.sharing.ocm.v1beta1.OcmAPI/GetOCMShareByToken"}
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

func (s *service) webdavURL(ctx context.Context, share *ocm.Share) string {
	// the url is in the form of https://cernbox.cern.ch/remote.php/dav/ocm/token
	p, _ := url.JoinPath(s.conf.WebDAVEndpoint, "/remote.php/dav/ocm", share.Token)
	return p
}

func (s *service) getWebdavProtocol(ctx context.Context, share *ocm.Share, m *ocm.AccessMethod_WebdavOptions) *ocmd.WebDAV {
	var perms []string
	if m.WebdavOptions.Permissions.InitiateFileDownload {
		perms = append(perms, "read")
	}
	if m.WebdavOptions.Permissions.InitiateFileUpload {
		perms = append(perms, "write")
	}

	return &ocmd.WebDAV{
		Permissions:  perms,
		URL:          s.webdavURL(ctx, share),
		SharedSecret: share.Token,
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

func (s *service) getDataTransferProtocol(ctx context.Context, share *ocm.Share) *ocmd.Datatx {
	var size uint64
	// get the path of the share
	statRes, err := s.gateway.Stat(ctx, &providerpb.StatRequest{
		Ref: &providerpb.Reference{
			ResourceId: share.ResourceId,
		},
	})
	if err != nil {
		panic(err)
	}

	path := statRes.GetInfo().Path
	err = s.walk(ctx, path, func(path string, info *providerpb.ResourceInfo, err error) error {
		if info.Type == providerpb.ResourceType_RESOURCE_TYPE_FILE {
			size += info.Size
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return &ocmd.Datatx{
		SourceURI: s.webdavURL(ctx, share),
		Size:      size,
	}
}

// walk traverses the path recursively to discover all resources in the tree.
func (s *service) walk(ctx context.Context, path string, fn walker.WalkFunc) error {
	return s.walker.Walk(ctx, path, fn)
}

func (s *service) getProtocols(ctx context.Context, share *ocm.Share) ocmd.Protocols {
	var p ocmd.Protocols
	for _, m := range share.AccessMethods {
		switch t := m.Term.(type) {
		case *ocm.AccessMethod_WebdavOptions:
			p = append(p, s.getWebdavProtocol(ctx, share, t))
		case *ocm.AccessMethod_WebappOptions:
			p = append(p, s.getWebappProtocol(share))
		case *ocm.AccessMethod_TransferOptions:
			p = append(p, s.getDataTransferProtocol(ctx, share))
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
	user := appctx.ContextMustGetUser(ctx)
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
		ProviderID: ocmshare.Id.OpaqueId,
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
		Protocols:         s.getProtocols(ctx, ocmshare),
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
	user := appctx.ContextMustGetUser(ctx)
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
	// if the request is by token, the user does not need to be in the ctx
	var user *userpb.User
	if req.Ref.GetToken() == "" {
		user = appctx.ContextMustGetUser(ctx)
	}
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

func (s *service) GetOCMShareByToken(ctx context.Context, req *ocm.GetOCMShareByTokenRequest) (*ocm.GetOCMShareByTokenResponse, error) {
	ocmshare, err := s.repo.GetShare(ctx, nil, &ocm.ShareReference{
		Spec: &ocm.ShareReference_Token{
			Token: req.Token,
		},
	})
	if err != nil {
		if errors.Is(err, share.ErrShareNotFound) {
			return &ocm.GetOCMShareByTokenResponse{
				Status: status.NewNotFound(ctx, "share does not exist"),
			}, nil
		}
		return &ocm.GetOCMShareByTokenResponse{
			Status: status.NewInternal(ctx, err, "error getting share"),
		}, nil
	}

	return &ocm.GetOCMShareByTokenResponse{
		Status: status.NewOK(ctx),
		Share:  ocmshare,
	}, nil
}

func (s *service) ListOCMShares(ctx context.Context, req *ocm.ListOCMSharesRequest) (*ocm.ListOCMSharesResponse, error) {
	user := appctx.ContextMustGetUser(ctx)
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
	user := appctx.ContextMustGetUser(ctx)
	if len(req.Field) == 0 {
		return &ocm.UpdateOCMShareResponse{
			Status: status.NewOK(ctx),
		}, nil
	}
	_, err := s.repo.UpdateShare(ctx, user, req.Ref, req.Field...)
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
	user := appctx.ContextMustGetUser(ctx)
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
	user := appctx.ContextMustGetUser(ctx)
	_, err := s.repo.UpdateReceivedShare(ctx, user, req.Share, req.UpdateMask)
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
	user := appctx.ContextMustGetUser(ctx)
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
