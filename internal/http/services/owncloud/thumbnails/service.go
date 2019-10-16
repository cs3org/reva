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

package thumbnails

import (
	"net/http"
	"os"

	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/mitchellh/mapstructure"
)

func init() {
	rhttp.Register("thumbnails", New)
}

// New returns a new thumbnail service
func New(m map[string]interface{}) (rhttp.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}
	if conf.Prefix == "" {
		conf.Prefix = "thumbnails"
	}
	return &svc{conf: conf, Cache: make(map[string]bool)}, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	cache, _ := cacheFolder()
	os.RemoveAll(cache)
	return nil
}

type config struct {
	Prefix     string `mapstructure:"prefix"`
	WebDavHost string `mapstructure:"webdav_host"`
}

type svc struct {
	conf  *config
	Cache map[string]bool
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := newThumbnail(s, w, r)
		if handler.CheckCache() {
			return
		}
		httpRes := handler.GetFile()
		if httpRes == nil {
			return
		}
		handler.GenerateThumbnail(httpRes)
		httpRes.Body.Close()
	})
}
