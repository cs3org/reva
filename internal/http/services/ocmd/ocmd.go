// Copyright 2018-2020 CERN
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

package ocmd

import (
	"net/http"

	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rhttp/router"
	"github.com/mitchellh/mapstructure"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds the config options that need to be passed down to all ocdav handlers
type Config struct {
	Prefix string `mapstructure:"prefix"`
	Host   string `mapstructure:"host"`
}

type svc struct {
	Conf               *Config
	ProviderAuthorizer *providerAuthorizer
	ShareManager       *shareManager
}

// New returns a new ocmd object
func New(m map[string]interface{}) (global.Service, error) {
	conf := &Config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	s := &svc{
		Conf: conf,
	}
	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.Conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{"/"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := appctx.GetLogger(ctx)
		var head string
		head, r.URL.Path = router.ShiftPath(r.URL.Path)
		log.Debug().Str("head", head).Str("tail", r.URL.Path).Msg("http routing")
		switch head {
		case "ocm-provider":
			s.getOCMInfo(log, s.Conf.Host).ServeHTTP(w, r)
			return
		case "shares":
			s.addShare(log, *s.ShareManager, *s.ProviderAuthorizer).ServeHTTP(w, r)
			return
		case "notifications":
			s.notImplemented(log).ServeHTTP(w, r)
			return
		case "webdav":
			s.proxyWebdav(log, *s.ShareManager, *s.ProviderAuthorizer).ServeHTTP(w, r)
			return
		case "internal/shares":
			s.propagateInternalShare(log, *s.ShareManager, *s.ProviderAuthorizer).ServeHTTP(w, r)
			return
		case "internal/providers":
			s.addProvider(log, *s.ProviderAuthorizer).ServeHTTP(w, r)
			return
		case "metrics":
			promhttp.Handler().ServeHTTP(w, r)
			return

		}
		log.Warn().Msg("resource not found")
		w.WriteHeader(http.StatusNotFound)

	})
}
