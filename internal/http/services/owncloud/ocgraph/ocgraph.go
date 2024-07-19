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

// This package implements the APIs defined in https://owncloud.dev/apis/http/graph/

package ocgraph

import (
	"context"
	"net/http"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/cfg"
)

func init() {
	global.Register("ocgraph", New)
}

type config struct {
	GatewaySvc string `mapstructure:"gatewaysvc"  validate:"required"`
	WebDavBase string `mapstructure:"webdav_base" validate:"required"`
	WebBase    string `mapstructure:"web_base"    validate:"required"`
}

func (c *config) ApplyDefaults() {
	c.GatewaySvc = sharedconf.GetGatewaySVC(c.GatewaySvc)
}

type svc struct {
	c *config
}

func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
	var c config
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}

	s := &svc{
		c: &c,
	}

	return s, nil
}

func (s *svc) getClient() (gateway.GatewayAPIClient, error) {
	return pool.GetGatewayServiceClient(pool.Endpoint(s.c.GatewaySvc))
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)

		if head == "v1.0" {
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			switch head {
			case "drives":
				s.getSpace(w, r)
				return
			case "me":
				head, r.URL.Path = router.ShiftPath(r.URL.Path)
				switch head {
				case "":
					s.getMe(w, r)
					return
				case "drives":
					s.listMySpaces(w, r)
					return
				}
			}
		} else if head == "v1beta1" {
			head, r.URL.Path = router.ShiftPath(r.URL.Path)
			// https://demo.owncloud.com/graph/v1beta1/me/drive/sharedWithMe
			switch head {
			case "me":
				head, r.URL.Path = router.ShiftPath(r.URL.Path)
				switch head {
				case "drive":
					head, r.URL.Path = router.ShiftPath(r.URL.Path)
					switch head {
					case "sharedWithMe":
						s.getSharedWithMe(w, r)
						return
					}
				}
			}
		}

		w.WriteHeader(http.StatusNotFound)
	})
}

func (s *svc) Prefix() string { return "graph" }

func (s *svc) Close() error { return nil }

func (s *svc) Unprotected() []string { return nil }
