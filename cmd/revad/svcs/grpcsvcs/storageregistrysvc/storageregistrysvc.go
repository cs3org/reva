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

package storageregistrysvc

import (
	"context"
	"fmt"

	storagetypespb "github.com/cernbox/go-cs3apis/cs3/storagetypes"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	"google.golang.org/grpc"

	storageregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageregistry/v0alpha"
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	"github.com/cernbox/reva/pkg/storage"
	"github.com/cernbox/reva/pkg/storage/broker/registry"
	"github.com/mitchellh/mapstructure"
)

var logger = log.New("storageregistrysvc")
var errors = err.New("storageregistrysvc")

func init() {
	grpcserver.Register("storageregistrysvc", New)
}

type service struct {
	broker storage.Broker
}

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

// New creates a new StorageBrokerService
func New(m map[string]interface{}, ss *grpc.Server) error {
	c, err := parseConfig(m)
	if err != nil {
		return err
	}

	broker, err := getBroker(c)
	if err != nil {
		return err
	}

	service := &service{
		broker: broker,
	}

	storageregistryv0alphapb.RegisterStorageRegistryServiceServer(ss, service)
	return nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func getBroker(c *config) (storage.Broker, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *service) ListStorageProviders(ctx context.Context, req *storageregistryv0alphapb.ListStorageProvidersRequest) (*storageregistryv0alphapb.ListStorageProvidersResponse, error) {
	var providers []*storagetypespb.ProviderInfo
	pinfos, err := s.broker.ListProviders(ctx)
	if err != nil {
		res := &storageregistryv0alphapb.ListStorageProvidersResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	for _, info := range pinfos {
		providers = append(providers, format(info))
	}

	res := &storageregistryv0alphapb.ListStorageProvidersResponse{
		Status:    &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Providers: providers,
	}
	return res, nil
}

func (s *service) GetStorageProvider(ctx context.Context, req *storageregistryv0alphapb.GetStorageProviderRequest) (*storageregistryv0alphapb.GetStorageProviderResponse, error) {
	fn := req.Ref.GetPath()
	p, err := s.broker.FindProvider(ctx, fn)
	if err != nil {
		logger.Error(ctx, err)
		res := &storageregistryv0alphapb.GetStorageProviderResponse{
			Status: &rpcpb.Status{Code: rpcpb.Code_CODE_INTERNAL},
		}
		return res, nil
	}

	provider := format(p)
	res := &storageregistryv0alphapb.GetStorageProviderResponse{
		Status:   &rpcpb.Status{Code: rpcpb.Code_CODE_OK},
		Provider: provider,
	}
	return res, nil
}

// TODO(labkode): fix
func format(p *storage.ProviderInfo) *storagetypespb.ProviderInfo {
	return &storagetypespb.ProviderInfo{
		Address:      p.Endpoint,
		ProviderPath: p.MountPath,
		//ProviderId: p.?
	}
}
