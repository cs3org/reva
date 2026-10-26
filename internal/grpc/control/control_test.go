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
	"fmt"
	"slices"
	"testing"

	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
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

// TestListInvocationsDescribesArgs checks that the argument types of a typed
// invocation reach the wire, so that the CLI can check and complete them.
func TestListInvocationsDescribesArgs(t *testing.T) {
	const id = "127.0.0.1:9002/typed"
	set := invoke.NewSet()
	set.Add("op", "").Handle(invoke.Typed(func(context.Context, struct {
		Level string   `arg:"level" enum:"info,warn" default:"info" help:"the level"`
		IDs   []string `arg:"ids,required"`
	}) (invoke.Result, error) {
		return nil, nil
	}))
	invoke.RegisterInstance(id, "typed", nil, set, nil)

	list, err := NewServer().ListInvocations(context.Background(), &controlpb.ListInvocationsRequest{Target: id})
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(list.Invocations, func(s *controlpb.InvocationSpec) bool { return s.Name == "op" })
	if i < 0 {
		t.Fatalf("no op in %+v", list.Invocations)
	}
	want := []*controlpb.ArgSpec{
		{Name: "level", Description: "the level", Type: "string", Default: "info", Enum: []string{"info", "warn"}},
		{Name: "ids", Required: true, Type: "string", Repeated: true},
	}
	if got := list.Invocations[i].Args; len(got) != len(want) || !proto.Equal(got[0], want[0]) || !proto.Equal(got[1], want[1]) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestErrCode(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want codes.Code
	}{
		{errtypes.NotFound("x"), codes.NotFound},
		{fmt.Errorf("wrapped: %w", errtypes.NotFound("x")), codes.NotFound},
		{errtypes.BadRequest("x"), codes.InvalidArgument},
		{errtypes.PermissionDenied("x"), codes.PermissionDenied},
		{errtypes.NotSupported("x"), codes.Unimplemented},
		{errtypes.AlreadyExists("x"), codes.AlreadyExists},
		{status.Error(codes.FailedPrecondition, "x"), codes.FailedPrecondition},
		{invoke.Error{Code: codes.ResourceExhausted, Message: "x"}, codes.ResourceExhausted},
		{errors.New("x"), codes.Unknown},
	} {
		if got := errCode(tc.err); got != tc.want {
			t.Errorf("errCode(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
