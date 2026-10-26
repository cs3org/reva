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
	"errors"
	"net"
	"path/filepath"
	"slices"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/control/controlpb"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/invoke/client"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func spaceRef(name string) *adminpb.SpaceRef {
	return &adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Name{Name: name}}
}

// facadeEndpoints serves a fake service over a real control channel, and
// returns its endpoints.
func facadeEndpoints(t *testing.T) []client.Endpoint {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	controlpb.RegisterControlServer(srv, control.NewServer())
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	set := invoke.NewSet()
	set.Add("get", "").Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) {
		return invoke.ProtoResult(&adminpb.SpaceRef{Ref: &adminpb.SpaceRef_Id{Id: &provider.StorageSpaceId{OpaqueId: "s1"}}})
	}))
	set.Add("missing", "").Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) {
		return nil, invoke.Error{Code: codes.NotFound, Message: "no such space"}
	}))
	set.Add("plain", "").Handle(invoke.Typed(func(context.Context, struct{}) (invoke.Result, error) {
		return nil, errors.New("boom")
	}))
	set.Add("list", "").HandleStream(invoke.TypedStream(func(_ context.Context, _ struct{}, emit invoke.StreamEmit) error {
		for _, n := range []string{"a", "b", "c"} {
			r, err := invoke.ProtoResult(spaceRef(n))
			if err != nil {
				return err
			}
			if err := emit(r); err != nil {
				return err
			}
		}
		return nil
	}))
	set.Add("list_fail", "").HandleStream(invoke.TypedStream(func(_ context.Context, _ struct{}, emit invoke.StreamEmit) error {
		r, _ := invoke.ProtoResult(spaceRef("a"))
		if err := emit(r); err != nil {
			return err
		}
		return status.Error(codes.PermissionDenied, "not allowed")
	}))

	id := ln.Addr().String() + "/facadetest"
	invoke.RegisterInstance(id, "facadetest", nil, set, nil)
	reg, err := registry.New("memory", nil, registry.Thresholds{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(registry.NewService("facadetest", []registry.Node{
		registry.NewNode(id, ln.Addr().String(), map[string]string{
			registry.MetaState: registry.StateReady, registry.MetaControl: ln.Addr().String(),
		}),
	})); err != nil {
		t.Fatal(err)
	}
	_, eps, err := client.Resolve(reg, "facadetest")
	if err != nil {
		t.Fatal(err)
	}
	return eps
}

func TestInvokeOneDecodesTheResult(t *testing.T) {
	eps := facadeEndpoints(t)
	res, err := invokeOne(context.Background(), eps, "get", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got adminpb.SpaceRef
	if err := decode(res.ResultJSON, &got); err != nil {
		t.Fatal(err)
	}
	if got.GetId().GetOpaqueId() != "s1" {
		t.Fatalf("unexpected result %v", &got)
	}
}

// The code of an invocation error reaches the caller of the typed RPC.
func TestInvocationErrorCodes(t *testing.T) {
	eps := facadeEndpoints(t)
	for invocation, want := range map[string]codes.Code{"missing": codes.NotFound, "plain": codes.Unknown} {
		_, err := invokeOne(context.Background(), eps, invocation, nil)
		if status.Code(err) != want {
			t.Errorf("%s: got %v, want %v", invocation, err, want)
		}
		if _, err := invokeAll(context.Background(), eps, invocation, nil); status.Code(err) != want {
			t.Errorf("%s on every instance: got %v, want %v", invocation, err, want)
		}
	}
	// The generic Invoke reports the code too.
	if rs := fanOutInvoke(context.Background(), eps, "missing", nil); codes.Code(rs[0].Code) != codes.NotFound {
		t.Errorf("Invoke: got code %v, want NotFound", rs[0].Code)
	}
}

func TestRelayStream(t *testing.T) {
	eps := facadeEndpoints(t)
	var got []string
	send := func(m *adminpb.SpaceRef) error { got = append(got, m.GetName()); return nil }

	if err := relayStream(context.Background(), eps, "list", nil, send); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected items %v", got)
	}

	got = nil
	if err := relayStream(context.Background(), eps, "list_fail", nil, send); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
}

// The gRPC listener and the local socket serve the same services.
func TestTheSocketServesEveryService(t *testing.T) {
	if !localRootSupported() {
		t.Skip("local root needs Linux")
	}
	s := &svc{socket: filepath.Join(t.TempDir(), "admin.sock")}
	if err := s.startSocket(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.stopSocket)
	listener := grpc.NewServer()
	s.Register(listener)

	names := func(srv *grpc.Server) []string {
		var out []string
		for name := range srv.GetServiceInfo() {
			out = append(out, name)
		}
		slices.Sort(out)
		return out
	}
	want := []string{
		"reva.admin.v1beta1.AdminAPI", "reva.admin.v1beta1.SharesAdmin",
		"reva.admin.v1beta1.SpacesAdmin", "reva.admin.v1beta1.StorageAdmin",
	}
	if got := names(listener); !slices.Equal(got, want) {
		t.Fatalf("listener serves %v, want %v", got, want)
	}
	if got := names(s.socketServer); !slices.Equal(got, want) {
		t.Fatalf("socket serves %v, want %v", got, want)
	}
}

