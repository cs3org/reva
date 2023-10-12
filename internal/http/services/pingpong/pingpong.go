// Copyright 2018-2023 CERN
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

package pingpong

import (
	"context"
	"net/http"
	"strings"

	"github.com/cs3org/reva/internal/grpc/services/pingpong/proto"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	global.Register("pingpong", New)
}

// New returns a new helloworld service.
func New(ctx context.Context, m map[string]interface{}) (global.Service, error) {
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
	Prefix   string `mapstructure:"prefix"`
	Endpoint string `mapstructure:"endpoint"`
}

func (c *config) ApplyDefaults() {
	if c.Prefix == "" {
		c.Prefix = "pingpong"
	}

	if c.Endpoint == "" {
		c.Endpoint = "localhost:8081"
	}
}

type svc struct {
	conf *config
}

func (s *svc) Prefix() string {
	return s.conf.Prefix
}

func (s *svc) Unprotected() []string {
	return []string{"/"}
}

func (s *svc) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, s.conf.Prefix)
		log := appctx.GetLogger(r.Context())
		log.Info().Msgf("action: %s", action)
		switch action {
		case "/ping":
			s.doPing(w, r)
			return
		case "/pong":
			s.doPong(w, r)
			return
		default:
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	})
}

func (s *svc) getClient() (proto.PingPongServiceClient, error) {
	conn, err := grpc.Dial(
		s.conf.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	if err != nil {
		return nil, err
	}

	client := proto.NewPingPongServiceClient(conn)
	return client, nil
}

// doPing will call the grpc Pong method.
func (s *svc) doPing(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	client, err := s.getClient()
	if err != nil {
		log.Error().Err(err).Msg("error getting grpc client")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	pingRes, err := client.Pong(r.Context(), &proto.PongRequest{})
	if err != nil {
		log.Error().Err(err).Msg("error doing grpc pong")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Info().Msg("pinging from http to grpc")
	_, err = w.Write([]byte(pingRes.Info))
	log.Error().Err(err).Msg("error writing res")
}

// doPong will be (http) called from grpc Pong.
func (s *svc) doPong(w http.ResponseWriter, r *http.Request) {
	log := appctx.GetLogger(r.Context())
	_, err := w.Write([]byte("pong"))
	log.Error().Err(err).Msg("error writing res")
}
