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

package prometheussvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/httpserver"

	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func init() {
	httpserver.Register("prometheussvc", New)
}

// New returns a new prometheus service
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	return &svc{prefix: conf.Prefix}, nil
}

type config struct {
	Prefix string `mapstructure:"prefix"`
}

type svc struct {
	prefix string
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return promhttp.Handler()
}

func (s *svc) Close() error {
	return nil
}
