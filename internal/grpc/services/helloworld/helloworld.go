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

package helloworld

import (
	"context"
	"fmt"

	"github.com/cs3org/reva/internal/grpc/services/helloworld/proto"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/utils/cfg"
	"google.golang.org/grpc"
)

func init() {
	rgrpc.Register("helloworld", New)
}

type conf struct {
	Message string `mapstructure:"message"`
}
type service struct {
	conf *conf
}

func (c *conf) ApplyDefaults() {
	if c.Message == "" {
		c.Message = "Hello"
	}
}

// New returns a new PreferencesServiceServer
// It can be tested like this:
// prototool grpc --address 0.0.0.0:9999 --method 'revad.helloworld.HelloWorldService/Hello' --data '{"name": "Alice"}'.
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
	return []string{"/revad.helloworld.HelloWorldService/Hello"}
}

func (s *service) Register(ss *grpc.Server) {
	proto.RegisterHelloWorldServiceServer(ss, s)
}

func (s *service) Hello(ctx context.Context, req *proto.HelloRequest) (*proto.HelloResponse, error) {
	if req.Name == "" {
		req.Name = "Mr. Nobody"
	}
	message := fmt.Sprintf("%s %s", s.conf.Message, req.Name)
	res := &proto.HelloResponse{
		Message: message,
	}
	return res, nil
}
