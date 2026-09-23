// Copyright 2018-2024 CERN
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

package helloworld

import (
	"context"
	"net/http"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the service is served.
const mount = "/helloworld"

func init() {
	global.Register("helloworld", New)
}

// New returns a new helloworld service.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	return &svc{conf: &c}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

type config struct {
	HelloMessage string `mapstructure:"message"`
}

func (c *config) ApplyDefaults() {
	if c.HelloMessage == "" {
		c.HelloMessage = "Hello World!"
	}
}

type svc struct {
	conf *config
}

func (s *svc) Prefix() string {
	return mount
}

func (s *svc) Routes(r *router.Router) {
	r.Any(mount, s.hello, router.Unprotected())
}

func (s *svc) hello(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	if _, err := w.Write([]byte(s.conf.HelloMessage)); err != nil {
		log.Err(err).Msg("error writing response")
	}
}
