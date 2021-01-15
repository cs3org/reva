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

package ocs

import (
	"net/http"

	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/config"
	"github.com/cs3org/reva/internal/http/services/owncloud/ocs/response"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

const (
	apiV1 = "v1.php"
	apiV2 = "v2.php"
)

func init() {
	global.Register("ocs", New)
}

type svc struct {
	c         *config.Config
	V1Handler *V1Handler
}

// New returns a new capabilitiessvc
func New(m map[string]interface{}, log *zerolog.Logger) (global.Service, error) {
	conf := &config.Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	conf.Init()

	s := &svc{
		c:         conf,
		V1Handler: new(V1Handler),
	}

	// initialize handlers and set default configs
	if err := s.V1Handler.init(conf); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *svc) Prefix() string {
	return s.c.Prefix
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log := appctx.GetLogger(r.Context())

		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)

		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("ocs routing")

		if !(head == apiV1 || head == apiV2) {
			response.WriteOCSError(w, r, response.MetaNotFound.StatusCode, "Not found", nil)
			return
		}
		ctx := response.WithAPIVersion(r.Context(), head)
		s.V1Handler.Handler().ServeHTTP(w, r.WithContext(ctx))
	})
}
