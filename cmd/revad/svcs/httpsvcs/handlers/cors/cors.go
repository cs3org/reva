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

const (
	defaultPriority = 200
)

func init() {
	httpserver.RegisterMiddleware("cors", New)
}

type config struct {
	AllowCredentials   bool     `mapstructure:"allow_credentials"`
	OptionsPassthrough bool     `mapstructure:"options_passthrough"`
	MaxAge             int      `mapstructure:"max_age"`
	Priority           int      `mapstructure:"priority"`
	AllowedMethods     []string `mapstructure:"allowed_methods"`
	AllowedHeaders     []string `mapstructure:"allowed_headers"`
	ExposedHeaders     []string `mapstructure:"exposed_headers"`
	AllowedOrigins     []string `mapstructure:"allowed_origins"`
}

// New creates a new CORS middleware.
func New(m map[string]interface{}) (httpserver.Middleware, int, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, 0, err
	}

	if conf.Priority == 0 {
		conf.Priority = defaultPriority
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
		// TODO(jfd): use log from request context, otherwise fmt will be used to log,
		// preventing us from pinging the log to eg jq
	})

	return c.Handler, conf.Priority, nil
}
