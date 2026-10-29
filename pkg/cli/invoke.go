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

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Invoke runs an invocation on one instance of service, and decodes its JSON
// result into out. A nil out ignores the result. service is a service name
// or a node id (host:port/service). For a name, Invoke takes one ready
// instance, because the Admin API runs the invocation on every instance of a
// name.
func (r *Runtime) Invoke(ctx context.Context, service, invocation string, args map[string]string, out any) error {
	c, ctx, err := r.Admin(ctx)
	if err != nil {
		return err
	}
	node, err := oneNode(ctx, c, service, invocation)
	if err != nil {
		return err
	}
	res, err := c.Invoke(ctx, &adminpb.InvokeRequest{Service: node, Invocation: invocation, Args: args})
	if err != nil {
		return err
	}
	if len(res.Results) != 1 {
		return fmt.Errorf("invocation %s on %s: %d results, expected 1", invocation, node, len(res.Results))
	}
	if err := nodeError(res.Results[0].Error, res.Results[0].Code); err != nil {
		return err
	}
	if out == nil || res.Results[0].ResultJson == "" {
		return nil
	}
	return json.Unmarshal([]byte(res.Results[0].ResultJson), out)
}

// InvokeStream runs a streaming invocation on one instance of service, as
// Invoke does, and calls each with every result until the stream ends.
func (r *Runtime) InvokeStream(ctx context.Context, service, invocation string, args map[string]string, each func(json.RawMessage) error) error {
	c, ctx, err := r.Admin(ctx)
	if err != nil {
		return err
	}
	node, err := oneNode(ctx, c, service, invocation)
	if err != nil {
		return err
	}
	stream, err := c.InvokeStream(ctx, &adminpb.InvokeRequest{Service: node, Invocation: invocation, Args: args})
	if err != nil {
		return err
	}
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := nodeError(msg.Error, msg.Code); err != nil {
			return err
		}
		if err := each(json.RawMessage(msg.ResultJson)); err != nil {
			return err
		}
	}
}

func oneNode(ctx context.Context, c *AdminClient, service, invocation string) (string, error) {
	if strings.Contains(service, "/") {
		return service, nil
	}
	res, err := c.ListServices(ctx, &adminpb.ListServicesRequest{Service: service, Invocation: invocation})
	if err != nil {
		return "", err
	}
	for _, s := range res.Services {
		if i := slices.IndexFunc(s.Nodes, func(n *adminpb.Node) bool { return n.State == "ready" }); i >= 0 {
			return s.Nodes[i].Id, nil
		}
	}
	return "", fmt.Errorf("no ready instance of %s has the invocation %s", service, invocation)
}

func nodeError(msg string, code int32) error {
	if msg == "" {
		return nil
	}
	c := codes.Code(code)
	if c == codes.OK {
		c = codes.Unknown
	}
	return status.Error(c, msg)
}
