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
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeInvoke struct {
	fakeAdmin
	invoked []string
}

func (f *fakeInvoke) ListServices(context.Context, *adminpb.ListServicesRequest) (*adminpb.ListServicesResponse, error) {
	return &adminpb.ListServicesResponse{Services: []*adminpb.Service{{Name: "otg", Nodes: []*adminpb.Node{
		{Id: "a:1/otg", State: "offline"}, {Id: "b:1/otg", State: "ready"}, {Id: "c:1/otg", State: "ready"},
	}}}}, nil
}

func (f *fakeInvoke) Invoke(_ context.Context, req *adminpb.InvokeRequest) (*adminpb.InvokeResponse, error) {
	f.mu.Lock()
	f.invoked = append(f.invoked, req.Service)
	f.mu.Unlock()
	if req.Invocation == "missing" {
		return &adminpb.InvokeResponse{Results: []*adminpb.NodeResult{{Node: req.Service, Error: "no message", Code: int32(codes.NotFound)}}}, nil
	}
	return &adminpb.InvokeResponse{Results: []*adminpb.NodeResult{{Node: req.Service, ResultJson: `{"message":"down"}`}}}, nil
}

// The Admin API runs an invocation on every instance of a service name, so
// Invoke must send a node id.
func TestInvokeRunsOnOneReadyInstance(t *testing.T) {
	setup(t)
	f := &fakeInvoke{adminToken: token(t, "alice", time.Now().Add(time.Hour))}
	rt := &Runtime{adminHost: serveTCP(t, f), insecure: true, tokenValue: token(t, "alice", time.Now().Add(time.Hour))}
	defer rt.Close()

	var out struct{ Message string }
	if err := rt.Invoke(context.Background(), "otg", "get", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.Message != "down" || len(f.invoked) != 1 || f.invoked[0] != "b:1/otg" {
		t.Errorf("out = %+v, invoked on %v: want one call on b:1/otg", out, f.invoked)
	}

	err := rt.Invoke(context.Background(), "c:1/otg", "missing", nil, nil)
	if status.Code(err) != codes.NotFound || f.invoked[1] != "c:1/otg" {
		t.Errorf("err = %v, invoked on %v: want NotFound from c:1/otg", err, f.invoked)
	}
}
