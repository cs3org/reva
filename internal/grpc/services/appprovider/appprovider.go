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

package appprovider

import (
	"context"
	"os"
	"time"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("appprovider", New)
}

type service struct {
	provider app.Provider
	conf     *config
}

type config struct {
	Driver         string                            `mapstructure:"driver"`
	Drivers        map[string]map[string]interface{} `mapstructure:"drivers"`
	AppProviderURL string                            `mapstructure:"app_provider_url"`
	GatewaySvc     string                            `mapstructure:"gatewaysvc"`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "demo"
	}
	c.AppProviderURL = sharedconf.GetGatewaySVC(c.AppProviderURL)
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, err
	}
	c.init()
	return c, nil
}

// New creates a new AppProviderService
func New(m map[string]interface{}, ss *grpc.Server) (rgrpc.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	provider, err := getProvider(c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:     c,
		provider: provider,
	}

	go service.registerProvider()
	return service, nil
}

func (s *service) registerProvider() {
	// Give the appregistry service time to come up
	time.Sleep(2 * time.Second)

	ctx := context.Background()
	log := logger.New().With().Int("pid", os.Getpid()).Logger()
	pInfo, err := s.provider.GetAppProviderInfo(ctx)
	if err != nil {
		log.Error().Err(err).Msgf("error registering app provider: could not get provider info")
		return
	}
	pInfo.Address = s.conf.AppProviderURL

	client, err := pool.GetGatewayServiceClient(s.conf.GatewaySvc)
	if err != nil {
		log.Error().Err(err).Msgf("error registering app provider: could not get gateway client")
		return
	}
	res, err := client.AddAppProvider(ctx, &registrypb.AddAppProviderRequest{Provider: pInfo})
	if err != nil {
		log.Error().Err(err).Msgf("error registering app provider: error calling add app provider")
		return
	}
	if res.Status.Code != rpc.Code_CODE_OK {
		err = status.NewErrorFromCode(res.Status.Code, "appprovider")
		log.Error().Err(err).Msgf("error registering app provider: add app provider returned error")
		return
	}
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{}
}

func (s *service) Register(ss *grpc.Server) {
	providerpb.RegisterProviderAPIServer(ss, s)
}

func getProvider(c *config) (app.Provider, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func (s *service) OpenInApp(ctx context.Context, req *providerpb.OpenInAppRequest) (*providerpb.OpenInAppResponse, error) {
	appURL, err := s.provider.GetAppURL(ctx, req.ResourceInfo, req.ViewMode, req.AccessToken)
	if err != nil {
		err := errors.Wrap(err, "appprovider: error calling GetAppURL")
		res := &providerpb.OpenInAppResponse{
			Status: status.NewInternal(ctx, err, "error getting app URL"),
		}
		return res, nil
	}
	res := &providerpb.OpenInAppResponse{
		Status: status.NewOK(ctx),
		AppUrl: appURL,
	}
	return res, nil

}

func (s *service) OpenFileInAppProvider(ctx context.Context, req *providerpb.OpenFileInAppProviderRequest) (*providerpb.OpenFileInAppProviderResponse, error) {
	return nil, errtypes.NotSupported("Deprecated")
}
