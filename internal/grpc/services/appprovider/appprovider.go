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

package appprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	providerpb "github.com/cs3org/go-cs3apis/cs3/app/provider/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/app/registry/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/app"
	"github.com/cs3org/reva/pkg/app/provider/registry"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/mime"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rgrpc/status"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"github.com/juliangruber/go-intersect"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("appprovider", New)
	plugin.RegisterNamespace("grpc.services.appprovider.drivers", func(name string, newFunc any) {
		var f registry.NewFunc
		utils.Cast(newFunc, &f)
		registry.Register(name, f)
	})
}

type service struct {
	provider app.Provider
	conf     *config
}

type config struct {
	Driver              string                            `mapstructure:"driver"`
	Drivers             map[string]map[string]interface{} `mapstructure:"drivers"`
	AppProviderURL      string                            `mapstructure:"app_provider_url"`
	GatewaySvc          string                            `mapstructure:"gatewaysvc"`
	MimeTypes           []string                          `docs:"nil;A list of mime types supported by this app."                                                              mapstructure:"mime_types"`
	CustomMimeTypesJSON string                            `docs:"nil;An optional mapping file with the list of supported custom file extensions and corresponding mime types." mapstructure:"custom_mime_types_json"`
	Priority            uint64                            `mapstructure:"priority"`
	Language            string                            `mapstructure:"language"`
}

func (c *config) ApplyDefaults() {
	if c.Driver == "" {
		c.Driver = "demo"
	}
	c.AppProviderURL = sharedconf.GetGatewaySVC(c.AppProviderURL)
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

// New creates a new AppProviderService.
func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	// read and register custom mime types if configured
	if err := registerMimeTypes(c.CustomMimeTypesJSON); err != nil {
		return nil, err
	}

	provider, err := getProvider(ctx, &c)
	if err != nil {
		return nil, err
	}

	service := &service{
		conf:     &c,
		provider: provider,
	}

	go service.registerProvider(ctx)
	return service, nil
}

func registerMimeTypes(mappingFile string) error {
	// TODO(lopresti) this function also exists in the storage provider, to be seen if we want to factor it out, though a
	// fileext <-> mimetype "service" would have to be served by the gateway for it to be accessible both by storage providers and app providers.
	if mappingFile != "" {
		f, err := os.ReadFile(mappingFile)
		if err != nil {
			return fmt.Errorf("appprovider: error reading the custom mime types file: +%v", err)
		}
		mimeTypes := map[string]string{}
		err = json.Unmarshal(f, &mimeTypes)
		if err != nil {
			return fmt.Errorf("appprovider: error unmarshalling the custom mime types file: +%v", err)
		}
		// register all mime types that were read
		for e, m := range mimeTypes {
			mime.RegisterMime(e, m)
		}
	}
	return nil
}

func (s *service) registerProvider(ctx context.Context) {
	// Give the appregistry service time to come up
	// TODO(lopresti) we should register the appproviders after all other microservices
	time.Sleep(3 * time.Second)

	log := appctx.GetLogger(ctx)
	pInfo, err := s.provider.GetAppProviderInfo(ctx)
	if err != nil {
		log.Error().Err(err).Msgf("error registering app provider: could not get provider info")
		return
	}
	pInfo.Address = s.conf.AppProviderURL

	if len(s.conf.MimeTypes) != 0 {
		mimeTypesIf := intersect.Simple(pInfo.MimeTypes, s.conf.MimeTypes)
		var mimeTypes []string
		for _, m := range mimeTypesIf {
			mimeTypes = append(mimeTypes, m.(string))
		}
		pInfo.MimeTypes = mimeTypes
		log.Info().Str("appprovider", s.conf.AppProviderURL).Interface("mimetypes", mimeTypes).Msg("appprovider supported mimetypes")
	}

	client, err := pool.GetGatewayServiceClient(pool.Endpoint(s.conf.GatewaySvc))
	if err != nil {
		log.Error().Err(err).Msgf("error registering app provider: could not get gateway client")
		return
	}
	req := &registrypb.AddAppProviderRequest{Provider: pInfo}

	if s.conf.Priority != 0 {
		req.Opaque = &types.Opaque{
			Map: map[string]*types.OpaqueEntry{
				"priority": {
					Decoder: "plain",
					Value:   []byte(strconv.FormatUint(s.conf.Priority, 10)),
				},
			},
		}
	}

	res, err := client.AddAppProvider(ctx, req)
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

func getProvider(ctx context.Context, c *config) (app.Provider, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		driverConf := c.Drivers[c.Driver]
		if c.MimeTypes != nil {
			// share the mime_types config entry to the drivers
			if driverConf == nil {
				driverConf = make(map[string]interface{})
			}
			driverConf["mime_types"] = c.MimeTypes
		}
		return f(ctx, driverConf)
	}
	return nil, errtypes.NotFound("driver not found: " + c.Driver)
}

func (s *service) OpenInApp(ctx context.Context, req *providerpb.OpenInAppRequest) (*providerpb.OpenInAppResponse, error) {
	appURL, err := s.provider.GetAppURL(ctx, req.ResourceInfo, req.ViewMode, req.AccessToken, req.Opaque.Map, s.conf.Language)
	if err != nil {
		res := &providerpb.OpenInAppResponse{
			Status: status.NewStatusFromErrType(ctx, "appprovider: error calling GetAppURL", err),
		}
		return res, nil
	}
	res := &providerpb.OpenInAppResponse{
		Status: status.NewOK(ctx),
		AppUrl: appURL,
	}
	return res, nil
}
