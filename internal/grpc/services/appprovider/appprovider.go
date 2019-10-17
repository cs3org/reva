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

package appprovider

import (
	"context"
	"fmt"
	"io"

	appproviderv0alphapb "github.com/cs3org/go-cs3apis/cs3/appprovider/v0alpha"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/demo"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("appprovider", New)
}

type service struct {
	provider app.Provider
}

type config struct {
	Driver string                 `mapstructure:"driver"`
	Demo   map[string]interface{} `mapstructure:"demo"`
}

// New creates a new StorageRegistryService
func New(m map[string]interface{}, ss *grpc.Server) (io.Closer, error) {

	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	provider, err := getProvider(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		provider: provider,
	}

	appproviderv0alphapb.RegisterAppProviderServiceServer(ss, service)
	return service, nil
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *service) Close() error {
	return nil
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
	iframeLocation, err := s.provider.GetIFrame(ctx, req.ResourceInfo.Id, req.AccessToken)
	if err != nil {
		err := errors.Wrap(err, "appprovidersvc: error calling GetIFrame")
		res := &appproviderv0alphapb.OpenResponse{
			Status: status.NewInternal(ctx, err, "error getting app's iframe"),
		}
		return res, nil
	}
	res := &appproviderv0alphapb.OpenResponse{
		Status:    status.NewOK(ctx),
		IframeUrl: iframeLocation,
	}
	return res, nil
}
