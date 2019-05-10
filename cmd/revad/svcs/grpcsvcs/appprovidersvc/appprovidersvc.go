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

package appprovidersvc

import (
	"context"
	"fmt"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/demo"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/mitchellh/mapstructure"
)

type service struct {
	provider app.Provider
}

type config struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}, ss *grpc.Server) error {

	c, err := parseConfig(m)
	if err != nil {
		return err
	}

	provider, err := getProvider(c)
	if err != nil {
		return err
	}

	service := &service{
		provider: provider,
	}

	appproviderv0alphapb.RegisterAppProviderServiceServer(ss, service)
	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getProvider(c *config) (app.Provider, error) {
	switch c.Driver {
	case "demo":
		return demo.New(c.Demo)
	default:
		return nil, fmt.Errorf("driver not found: %s", c.Driver)
	}
}
func (s *service) Open(ctx context.Context, req *appproviderv0alphapb.OpenRequest) (*appproviderv0alphapb.OpenResponse, error) {
	log := appctx.GetLogger(ctx)
	id := req.ResourceId
	token := req.AccessToken

	resID := &storage.ResourceID{OpaqueID: id.OpaqueId, StorageID: id.StorageId}

	iframeLocation, err := s.provider.GetIFrame(ctx, resID, token)
	if err != nil {
		log.Error().Err(err).Msg("error getting iframe")
		res := &appproviderv0alphapb.OpenResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}
	res := &appproviderv0alphapb.OpenResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		IframeUrl: iframeLocation,
	}
	return res, nil
}
