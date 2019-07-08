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

package cors

import (
	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/cors"
)

func init() {
	httpserver.RegisterMiddleware("cors", New)
}

type config struct {
	Priority           int      `mapstructure:"priority"`
	AllowedOrigins     []string `mapstructure:"allowed_origins"`
	AllowCredentials   bool     `mapstructure:"allow_credentials"`
	AllowedMethods     []string `mapstructure:"allowed_methods"`
	AllowedHeaders     []string `mapstructure:"allowed_headers"`
	ExposedHeaders     []string `mapstructure:"exposed_headers"`
	MaxAge             int      `mapstructure:"max_age"`
	OptionsPassthrough bool     `mapstructure:"options_passthrough"`
}

// New creates a new CORS middleware.
func New(m map[string]interface{}) (httpserver.Middleware, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	c := cors.New(cors.Options{
		AllowCredentials:   conf.AllowCredentials,
		AllowedHeaders:     conf.AllowedHeaders,
		AllowedMethods:     conf.AllowedMethods,
		AllowedOrigins:     conf.AllowedOrigins,
		ExposedHeaders:     conf.ExposedHeaders,
		MaxAge:             conf.MaxAge,
		OptionsPassthrough: conf.OptionsPassthrough,
		Debug:              false,
		// TODO use log from request context, otherwise fmt will be used to log,
		// preventing us from pinging the log to eg jq
	})

	return c.Handler, conf.Priority, nil
}
