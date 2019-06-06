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

package ocssvc

import (
	"net/http"

	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/mitchellh/mapstructure"
)

func init() {
	httpserver.Register("ocssvc", New)
}

// Config holds the config options that need to be passed down to all ocs handlers
type Config struct {
	Prefix                 string           `mapstructure:"prefix"`
	Config                 ConfigData       `mapstructure:"config"`
	Capabilities           CapabilitiesData `mapstructure:"capabilities"`
	PublicShareProviderSVC string           `mapstructure:"publicshareprovidersvc"`
	UserShareProviderSVC   string           `mapstructure:"usershareprovidersvc"`
}

type svc struct {
	c         *Config
	handler   http.Handler
	V1Handler *V1Handler
}

// New returns a new capabilitiessvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	s := &svc{
		c:         conf,
		V1Handler: new(V1Handler),
	}

	// initialize handlers and set default configs
	s.V1Handler.init(conf)

	s.setHandler()
	return s, nil
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}
func (s *svc) Close() error {
	return nil
}
func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		var head string
		head, r.URL.Path = httpsvcs.ShiftPath(r.URL.Path)

		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("ocs routing")

		if head == "v1.php" || head == "v2.php" { // TODO v1 vs v2 ... what is the difference?
			s.V1Handler.Handler().ServeHTTP(w, r)
			return
		}

		http.Error(w, "Not Found", http.StatusNotFound)
	})
}
