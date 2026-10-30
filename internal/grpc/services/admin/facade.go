// Copyright 2018-2026 CERN
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

package admin

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// serviceEndpoints resolves every live instance of a service. A service that is
// not live is Unavailable, not NotFound: the CLI reads NotFound as "the resource
// does not exist".
func (s *svc) serviceEndpoints(service string) ([]client.Endpoint, error) {
	reg, err := s.registryHandle()
	if err != nil {
		return nil, err
	}
	_, eps, err := client.Resolve(reg, service)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "admin: %v", err)
	}
	return eps, nil
}

// invokeOne runs an invocation on one picked instance. It picks the instance
// itself, not through the Shared scope: an instance that does not advertise
// the scope yet would run the call more than once.
func invokeOne(ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string) (client.NodeResult, error) {
	res := client.InvokeOne(admin.ForwardReason(ctx), client.Pick(ctx, eps), invocation, args)
	return res, nodeStatus(invocation, res.Code, res.Error)
}

// invokeAll runs an invocation on every instance. It returns the results with
// the status of the first instance that failed, for a caller that accepts a
// partial failure.
func invokeAll(ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string) ([]client.NodeResult, error) {
	rs := client.FanOut(admin.ForwardReason(ctx), eps, invocation, args)
	for _, r := range rs {
		if err := nodeStatus(invocation, r.Code, r.Error); err != nil {
			return rs, err
		}
	}
	return rs, nil
}

// relayStream sends each result of a streaming invocation, decoded into an M.
// The first instance that fails ends the relay, so that a partial list does not
// look complete.
func relayStream[M proto.Message](ctx context.Context, eps []client.Endpoint, invocation string, args map[string]string, send func(M) error) error {
	ctx, cancel := context.WithCancel(admin.ForwardReason(ctx))
	defer cancel()
	for it := range streamAll(ctx, client.Route(eps, invocation), invocation, args) {
		if err := nodeStatus(invocation, codes.Code(it.Code), it.Error); err != nil {
			return err
		}
		var zero M
		m := zero.ProtoReflect().New().Interface().(M)
		if err := decode(it.ResultJson, m); err != nil {
			return err
		}
		if err := send(m); err != nil {
			return err
		}
	}
	return nil
}

// decode reads an invocation result into m. It discards unknown fields, so a
// newer node can add fields during an upgrade.
func decode(resultJSON string, m proto.Message) error {
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(resultJSON), m); err != nil {
		return status.Errorf(codes.Internal, "admin: decoding %s: %v", m.ProtoReflect().Descriptor().FullName(), err)
	}
	return nil
}

// nodeStatus is the gRPC status of a failed invocation, or nil if msg is empty.
// A node that sends no code sends 0, which becomes Unknown.
func nodeStatus(invocation string, code codes.Code, msg string) error {
	if msg == "" {
		return nil
	}
	if code == codes.OK {
		code = codes.Unknown
	}
	return status.Errorf(code, "admin: %s: %s", invocation, msg)
}
