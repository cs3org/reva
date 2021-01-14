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

package providerauthorizer

import (
	"fmt"
	"net/http"

	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/ocm/provider"
	"github.com/cs3org/reva/pkg/ocm/provider/authorizer/registry"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/user"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/mitchellh/mapstructure"
)

type config struct {
	Driver  string                            `mapstructure:"driver"`
	Drivers map[string]map[string]interface{} `mapstructure:"drivers"`
}

func (c *config) init() {
	if c.Driver == "" {
		c.Driver = "json"
	}
}

func getDriver(c *config) (provider.Authorizer, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}

	return nil, fmt.Errorf("driver %s not found for provider authorizer", c.Driver)
}

// New returns a new HTTP middleware that verifies that the provider is registered in OCM.
func New(m map[string]interface{}, unprotected []string, ocmPrefix string) (global.Middleware, error) {

	if ocmPrefix == "" {
		ocmPrefix = "ocm"
	}

	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	conf.init()

	authorizer, err := getDriver(conf)
	if err != nil {
		return nil, err
	}

	handler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			ctx := r.Context()
			log := appctx.GetLogger(ctx)
			head, _ := router.ShiftPath(r.URL.Path)

			if r.Method == "OPTIONS" || head != ocmPrefix || utils.Skip(r.URL.Path, unprotected) {
				log.Info().Msg("skipping provider authorizer check for: " + r.URL.Path)
				h.ServeHTTP(w, r)
				return
			}

			err = authorizer.IsProviderAllowed(ctx, &ocmprovider.ProviderInfo{
				Domain: user.ContextMustGetUser(ctx).Id.Idp,
			})
			if err != nil {
				log.Error().Err(err).Msg("provider not registered in OCM")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			h.ServeHTTP(w, r)
		})
	}

	return handler, nil

}
