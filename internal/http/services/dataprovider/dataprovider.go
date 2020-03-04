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

package dataprovider

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	global.Register("dataprovider", New)
}

type config struct {
	Prefix    string                            `mapstructure:"prefix"`
	Driver    string                            `mapstructure:"driver"`
	TmpFolder string                            `mapstructure:"tmp_folder"`
	Drivers   map[string]map[string]interface{} `mapstructure:"drivers"`
}

type svc struct {
	conf    *config
	handler http.Handler
	storage storage.FS
}

// New returns a new datasvc
func New(m map[string]interface{}) (global.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	if conf.Prefix == "" {
		conf.Prefix = "data"
	}

	if conf.TmpFolder == "" {
		conf.TmpFolder = os.TempDir()
	}

	if err := os.MkdirAll(conf.TmpFolder, 0755); err != nil {
		return nil, errors.Wrap(err, "could not create tmp dir")
	}

	fs, err := getFS(conf)
	if err != nil {
		return nil, err
	}

	s := &svc{
		storage: fs,
		conf:    conf,
	}
	s.setHandler()
	return s, nil
}

// Close performs cleanup.
func (s *svc) Close() error {
	return nil
}

func (s *svc) Unprotected() []string {
	return []string{}
}

func getFS(c *config) (storage.FS, error) {
	if f, ok := registry.NewFuncs[c.Driver]; ok {
		return f(c.Drivers[c.Driver])
	}
	return nil, fmt.Errorf("driver not found: %s", c.Driver)
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "HEAD":
			addCorsHeader(w)
			w.WriteHeader(http.StatusOK)
			return
		case "GET":
			s.doGet(w, r)
			return
		case "PUT":
			s.doPut(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
	})
}

func addCorsHeader(res http.ResponseWriter) {
	headers := res.Header()
	headers.Set("Access-Control-Allow-Origin", "*")
	headers.Set("Access-Control-Allow-Headers", "Content-Type, Origin, Authorization")
	headers.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
}
