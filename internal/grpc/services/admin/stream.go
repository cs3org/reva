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
	"io"
	"strconv"
	"sync"

	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InvokeStream is the streaming twin of Invoke: it opens a control-channel
// stream to every resolved instance and multiplexes their results, each
// labelled with its node, until the client disconnects or every upstream ends.
func (s *svc) InvokeStream(req *adminpb.InvokeRequest, stream adminpb.AdminAPI_InvokeStreamServer) error {
	if req.Service == "" || req.Invocation == "" {
		return status.Error(codes.InvalidArgument, "admin: service and invocation are required")
	}
	ctx := stream.Context()

	reg, err := s.registryHandle()
	if err != nil {
		return err
	}
	_, eps, err := resolveSelector(reg, req.Service)
	if err != nil {
		return status.Errorf(codes.NotFound, "admin: resolving %q: %v", req.Service, err)
	}

	admin.Audit(ctx, admin.AuditEvent{Action: "invoke-stream", Actor: actorName(ctx), Target: req.Service,
		Fields: map[string]string{"invocation": req.Invocation, "selector": req.Service, "instances": strconv.Itoa(len(eps))}})

	// One goroutine per endpoint feeds a shared channel, closed when all are
	// done.
	items := make(chan *adminpb.InvokeStreamResponse)
	var wg sync.WaitGroup
	for _, ep := range eps {
		wg.Add(1)
		go func(ep endpoint) {
			defer wg.Done()
			streamUpstream(ctx, ep, req.Invocation, req.Args, items)
		}(ep)
	}
	go func() { wg.Wait(); close(items) }()

	for {
		select {
		case <-ctx.Done():
			return nil
		case it, ok := <-items:
			if !ok {
				return nil
			}
			if err := stream.Send(it); err != nil {
				return err
			}
		}
	}
}

// streamUpstream forwards one endpoint's stream, node-labelled, into items. An
// unreachable endpoint yields a single per-node error item rather than failing
// the whole fan-in.
func streamUpstream(ctx context.Context, ep endpoint, invocation string, args map[string]string, items chan<- *adminpb.InvokeStreamResponse) {
	send := func(it *adminpb.InvokeStreamResponse) bool {
		select {
		case items <- it:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if ep.err != "" {
		send(&adminpb.InvokeStreamResponse{Node: ep.node, Error: ep.err})
		return
	}
	cli, err := controlClientAt(ep.addr)
	if err != nil {
		send(&adminpb.InvokeStreamResponse{Node: ep.node, Error: err.Error()})
		return
	}
	up, err := cli.InvokeStream(ctx, &controlpb.InvokeRequest{Target: ep.target, Invocation: invocation, Args: args})
	if err != nil {
		send(&adminpb.InvokeStreamResponse{Node: ep.node, Error: err.Error()})
		return
	}
	for {
		msg, err := up.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			// A cancelled client is a clean stop, not an error worth surfacing.
			if ctx.Err() == nil {
				send(&adminpb.InvokeStreamResponse{Node: ep.node, Error: err.Error()})
			}
			return
		}
		if !send(&adminpb.InvokeStreamResponse{Node: ep.node, ResultJson: msg.ResultJson, Error: msg.Error}) {
			return
		}
	}
}
