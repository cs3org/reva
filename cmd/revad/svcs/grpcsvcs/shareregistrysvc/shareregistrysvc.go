// Copyright 2018-2019 CERN
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

package shareregistrysvc

import (
	"context"
	"fmt"
	"io"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	shareregistryv0alphapb "github.com/cs3org/go-cs3apis/cs3/shareregistry/v0alpha"
	sharetypespb "github.com/cs3org/go-cs3apis/cs3/sharetypes"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/share"
	"github.com/cs3org/reva/pkg/share/registry"
	"github.com/mitchellh/mapstructure"
)

func init() {
	grpcserver.Register("shareregistrysvc", New)
}

type service struct {
	r share.Registry
}

func (s *service) Close() error {
	return nil
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

// New creates a new ShareRegistryService
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	r, err := getShareRegistry(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		r: r,
	}

	shareregistryv0alphapb.RegisterShareRegistryServiceServer(ss, service)
	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getShareRegistry(c *config) (share.Registry, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) ListShareProviders(ctx context.Context, req *shareregistryv0alphapb.ListShareProvidersRequest) (*shareregistryv0alphapb.ListShareProvidersResponse, error) {
	var providers []*sharetypespb.ProviderInfo
	pinfos, err := s.r.ListProviders(ctx)
	if err != nil {
		res := &shareregistryv0alphapb.ListShareProvidersResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	for _, info := range pinfos {
		providers = append(providers, format(info))
	}

	res := &shareregistryv0alphapb.ListShareProvidersResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Providers: providers,
	}
	return res, nil
}

func (s *service) GetShareProvider(ctx context.Context, req *shareregistryv0alphapb.GetShareProviderRequest) (*shareregistryv0alphapb.GetShareProviderResponse, error) {
	log := appctx.GetLogger(ctx)
	sType := req.GetShareType().String()
	p, err := s.r.FindProvider(ctx, sType)
	if err != nil {
		log.Error().Err(err).Msg("error finding provider")
		res := &shareregistryv0alphapb.GetShareProviderResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &shareregistryv0alphapb.GetShareProviderResponse{
		Status:   &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Provider: provider,
	}
	return res, nil
}

func format(p *share.ProviderInfo) *sharetypespb.ProviderInfo {
	// TODO(jfd) isnt there a protobuf thing to map this?
	var shareType sharetypespb.ShareType
	switch p.Type {
	case "SHARE_TYPE_USER":
		shareType = sharetypespb.ShareType_SHARE_TYPE_USER
	case "SHARE_TYPE_GROUP":
		shareType = sharetypespb.ShareType_SHARE_TYPE_GROUP
	case "SHARE_TYPE_PUBLIC_LINK":
		shareType = sharetypespb.ShareType_SHARE_TYPE_PUBLIC_LINK
	case "SHARE_TYPE_OCM":
		shareType = sharetypespb.ShareType_SHARE_TYPE_OCM
	default:
		shareType = sharetypespb.ShareType_SHARE_TYPE_INVALID
	}
	return &sharetypespb.ProviderInfo{
		Address:   p.Endpoint,
		ShareType: shareType,
	}
}
