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
	"crypto/tls"
	"io/ioutil"
	"net/http"

	"github.com/cs3org/reva/internal/grpc/services/pingpong/proto"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/httpclient"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("pingpong", New)
}

type conf struct {
	Endpoint string
}

type service struct {
	conf *conf
	*proto.UnimplementedPingPongServiceServer
}

func (c *conf) ApplyDefaults() {
	if c.Endpoint == "" {
		c.Endpoint = "http://localhost:8080/pingpong"
	}
}

func New(ctx context.Context, m map[string]interface{}) (rgrpc.Service, error) {
	var c conf
	if err := cfg.Decode(m, &c); err != nil {
		return nil, err
	}
	service := &service{conf: &c}
	return service, nil
}

func (s *service) Close() error {
	return nil
}

func (s *service) UnprotectedEndpoints() []string {
	return []string{"/revad.pingpong.PingPongService/Ping", "/revad.pingpong.PingPongService/Pong"}
}

func (s *service) Register(ss *grpc.Server) {
	proto.RegisterPingPongServiceServer(ss, s)
}

func (s *service) Ping(ctx context.Context, _ *proto.PingRequest) (*proto.PingResponse, error) {
	log := appctx.GetLogger(ctx)

	// we call the http Pong method
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := httpclient.New(httpclient.RoundTripper(tr))
	req, err := http.NewRequestWithContext(ctx, "GET", s.conf.Endpoint+"/pong", nil)
	if err != nil {
		log.Error().Err(err).Msg("error creating http request")
		return nil, err
	}

	res, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("eror doing http pong")
		return nil, err
	}

	defer res.Body.Close()
	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Error().Err(err).Msg("error reading response body")
		return nil, err
	}

	return &proto.PingResponse{Info: string(data)}, nil
}

func (s *service) Pong(_ context.Context, _ *proto.PongRequest) (*proto.PongResponse, error) {
	res := &proto.PongResponse{Info: "pong"}
	return res, nil
}
