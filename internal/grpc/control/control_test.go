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

package control

import (
	"context"
	"errors"
	"testing"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeInvokable struct{}

func (fakeInvokable) Invocations() []invoke.InvocationSpec {
	return []invoke.InvocationSpec{
		{Name: "dump", Description: "dump state", Kind: invoke.KindReadonly},
	}
}

func (fakeInvokable) Invoke(_ context.Context, name string, args map[string]any) (invoke.Result, error) {
	if name != "dump" {
		return nil, errors.New("unknown invocation")
	}
	return invoke.Result{"echo": args["k"]}, nil
}

// TestControlRouting checks that the control channel routes to the target it
// addresses (a node id here), rejects unknown targets, and surfaces invocation
// errors as soft per-node errors.
func TestControlRouting(t *testing.T) {
	const id = "127.0.0.1:9001/fake"
	invoke.RegisterInstance(id, "fake", nil, fakeInvokable{}, nil)
	ctrl := NewServer()
	ctx := context.Background()

	list, err := ctrl.ListInvocations(ctx, &controlpb.ListInvocationsRequest{Target: id})
	if err != nil {
		t.Fatalf("ListInvocations: %v", err)
	}
	// The instance exposes the shared config default plus its own "dump".
	names := map[string]bool{}
	for _, s := range list.Invocations {
		names[s.Name] = true
	}
	if !names["config"] || !names["dump"] {
		t.Fatalf("expected config + dump, got %+v", list.Invocations)
	}

	res, err := ctrl.Invoke(ctx, &controlpb.InvokeRequest{Target: id, Invocation: "dump", Args: map[string]string{"k": "v"}})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.Error != "" {
		t.Fatalf("unexpected soft error: %s", res.Error)
	}
	if res.ResultJson != `{"echo":"v"}` {
		t.Fatalf("unexpected result: %s", res.ResultJson)
	}

	// Unknown target on this node → gRPC NotFound.
	if _, err := ctrl.Invoke(ctx, &controlpb.InvokeRequest{Target: "1.2.3.4:9/missing", Invocation: "dump"}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for unknown target, got %v", err)
	}

	// Invocation-level failure → soft per-node error, not a gRPC error.
	res, err = ctrl.Invoke(ctx, &controlpb.InvokeRequest{Target: id, Invocation: "nope"})
	if err != nil {
		t.Fatalf("Invoke(nope): unexpected gRPC error %v", err)
	}
	if res.Error == "" {
		t.Fatal("expected soft error for unknown invocation")
	}
}
