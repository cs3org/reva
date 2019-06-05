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

package preferencessvc

import (
	"net/http"

	preferencesv0alphapb "github.com/cs3org/go-cs3apis/cs3/preferences/v0alpha"
	"github.com/cs3org/reva/cmd/revad/httpserver"
	"github.com/cs3org/reva/cmd/revad/svcs/httpsvcs"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

func init() {
	httpserver.Register("preferencessvc", New)
}

type config struct {
	Prefix         string `mapstructure:"prefix"`
	Preferencessvc string `mapstructure:"preferencessvc"`
}

type svc struct {
	prefix         string
	handler        http.Handler
	PreferencesSvc string
	conn           *grpc.ClientConn
	client         preferencesv0alphapb.PreferencesServiceClient
}

// New returns a new webuisvc
func New(m map[string]interface{}) (httpsvcs.Service, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	s := &svc{prefix: conf.Prefix,
		PreferencesSvc: conf.Preferencessvc,
	}
	s.setHandler()
	return s, nil
}

func (s *svc) Close() error {
	return nil
}

func (s *svc) Prefix() string {
	return s.prefix
}

func (s *svc) Handler() http.Handler {
	return s.handler
}

func (s *svc) setHandler() {
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.Method
		switch method {
		case "GET":
			s.doGet(w, r)
			return
		case "POST":
			s.doSet(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
	})
}

func (s *svc) getConn() (*grpc.ClientConn, error) {
	if s.conn != nil {
		return s.conn, nil
	}

	conn, err := grpc.Dial(s.PreferencesSvc, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (s *svc) getClient() (preferencesv0alphapb.PreferencesServiceClient, error) {
	if s.client != nil {
		return s.client, nil
	}

	conn, err := s.getConn()
	if err != nil {
		return nil, err
	}
	s.client = preferencesv0alphapb.NewPreferencesServiceClient(conn)
	return s.client, nil
}
