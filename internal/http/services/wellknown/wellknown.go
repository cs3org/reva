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

package wellknown

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/utils/cfg"
)

// mount is where the discovery documents are served.
const mount = "/.well-known"

func init() {
	global.Register("wellknown", New)
}

type svc struct {
	Conf *config
	ocm  *wkocmHandler
}

type config struct {
	OCMProvider OcmProviderConfig `mapstructure:"ocmprovider"`
}

// New returns a new wellknown object.
func New(ctx context.Context, m map[string]any) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{Conf: &c}
	s.ocm = new(wkocmHandler)
	s.ocm.init(&s.Conf.OCMProvider)

	return s, nil
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return mount
}

// Routes declares the discovery endpoints. They are public by definition:
// peers read them before they have any credentials.
func (s *svc) Routes(r *router.Router) {
	r.Get(mount+"/ocm", s.ocm.Ocm, router.Unprotected())
}
