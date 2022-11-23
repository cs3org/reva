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

package eventsmiddleware

import (
	"context"
	"fmt"

	"go-micro.dev/v4/util/log"
	"google.golang.org/grpc"

	"github.com/asim/go-micro/plugins/events/nats/v4"
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/pkg/events"
	"github.com/cs3org/reva/pkg/events/server"
	"github.com/cs3org/reva/pkg/rgrpc"
)

const (
	defaultPriority = 200
)

func init() {
	rgrpc.RegisterUnaryInterceptor("eventsmiddleware", NewUnary)
}

// NewUnary returns a new unary interceptor that emits events when needed
// no lint because of the switch statement that should be extendable
//

func NewUnary(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error) {
	publisher, err := publisherFromConfig(m)
	if err != nil {
		return nil, 0, err
	}

	interceptor := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		res, err := handler(ctx, req)
		if err != nil {
			return res, err
		}

		var ev interface{}

		// gocritic is disabled because the use of .(type) outside type switch is forbidden
		switch v := res.(type) { //nolint:gocritic
		case *collaboration.CreateShareResponse:
			ev = ShareCreated(v)
		}

		if ev != nil {
			if err := events.Publish(publisher, ev); err != nil {
				log.Error(err)
			}
		}

		return res, nil
	}
	return interceptor, defaultPriority, nil
}

// NewStream returns a new server stream interceptor
// that creates the application context.
func NewStream() grpc.StreamServerInterceptor {
	interceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// TODO: Use ss.RecvMsg() and ss.SendMsg() to send events from a stream
		return handler(srv, ss)
	}
	return interceptor
}

func publisherFromConfig(m map[string]interface{}) (events.Publisher, error) {
	typ := m["type"].(string)
	switch typ {
	default:
		return nil, fmt.Errorf("stream type '%s' not supported", typ)
	case "nats":
		address := m["address"].(string)
		cid := m["clusterID"].(string)
		return server.NewNatsStream(nats.Address(address), nats.ClusterID(cid))
	}
}
