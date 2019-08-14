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

package helloworldsvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("helloworldsvc", New)
}

// New returns a new helloworld service
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	if conf.HelloMessage == "" {
		conf.HelloMessage = "Hello World!"
	}
	return &svc{conf: conf}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	Prefix       string `mapstructure:"prefix"`
	HelloMessage string `mapstructure:"hello_message"`
}

type svc struct {
	conf *config
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())
		_, err := w.Write([]byte(s.conf.HelloMessage))
		log.Err(err).Msg("error writing response")
	})
}
