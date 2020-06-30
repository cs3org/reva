// Copyright 2018-2020 CERN
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

package meshdirectory

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	providerv1beta1 "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/internal/http/services/ocmd"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/mitchellh/mapstructure"
)

func init() {
	global.Register("meshdirectory", New)
}

type config struct {
	Driver     string                            `mapstructure:"driver"`
	Drivers    map[string]map[string]interface{} `mapstructure:"drivers"`
	Prefix     string                            `mapstructure:"prefix"`
	Static     string                            `mapstructure:"static"`
	GatewaySvc string                            `mapstructure:"gatewaysvc"`
}

func (c *config) init() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)

	if c.Prefix == "" {
		c.Prefix = "meshdir"
	}

	if c.Static == "" {
		c.Static = "static"
	}
}

type svc struct {
	conf *config
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new Mesh Directory HTTP service
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	c.init()

	service := &svc{
		conf: c,
	}
	return service, nil
}

// Service prefix
func (s *svc) Prefix() string {
	return s.conf.Prefix
}

// Unprotected endpoints
func (s *svc) Unprotected() []string {
	return []string{"/"}
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(s.conf.GatewaySvc)
}

func (s *svc) serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	file, err := ioutil.ReadFile(path.Clean(s.conf.Static + "/index.html"))
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error reading meshdirectory index page", err)
		log.Err(err).Msg("error reading meshdirectory index page")
		return
	}
	if _, err := w.Write(file); err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error rendering meshdirectory index page", err)
		log.Err(err).Msg("error rendering meshdirectory index page.")
		return
	}
}

// OCMProvidersOnly returns just the providers that provide the OCM Service Type endpoint
func (s *svc) OCMProvidersOnly(pi []*providerv1beta1.ProviderInfo) (po []*providerv1beta1.ProviderInfo) {
	for _, p := range pi {
		for _, s := range p.Services {
			if s.Endpoint.Type.Name == "OCM" {
				po = append(po, p)
				break
			}
		}
	}
	return
}

func (s *svc) serveJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := r.Context()
	log := appctx.GetLogger(ctx)

	gatewayClient, err := s.getClient()
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError,
			fmt.Sprintf("error getting grpc client on addr: %v", s.conf.GatewaySvc), err)
		log.Err(err).Msg(fmt.Sprintf("error getting grpc client on addr: %v", s.conf.GatewaySvc))
		return
	}

	providers, err := gatewayClient.ListAllProviders(ctx, &providerv1beta1.ListAllProvidersRequest{})
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error listing all providers", err)
		log.Err(err).Msg("error listing all mesh providers.")
		return
	}

	providers.Providers = s.OCMProvidersOnly(providers.Providers)
	jsonResponse, err := json.Marshal(providers.Providers)

	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error marshalling providers data", err)
		log.Err(err).Msg("error marshal providers data.")
		return
	}

	// Write response
	_, err = w.Write(jsonResponse)
	if err != nil {
		ocmd.WriteError(w, r, ocmd.APIErrorServerError, "error writing providers data", err)
		log.Err(err).Msg("error writing providers data.")
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HTTP service handler
func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")

		switch head {
		case "":
			s.serveIndex(w, r)
			return
		case "providers":
			s.serveJSON(w, r)
			return
		}
	})
}
