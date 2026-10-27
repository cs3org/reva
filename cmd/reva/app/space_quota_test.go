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

package revacmd

import (
	"context"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeQuotaAdmin knows one project, cernbox, with a replica factor of 2.
type fakeQuotaAdmin struct {
	adminpb.UnimplementedAdminAPIServer
	adminpb.UnimplementedSpacesAdminServer

	mu   sync.Mutex
	sets []*adminpb.SetSpaceQuotaRequest
	min  uint64
}

func (f *fakeQuotaAdmin) CheckAdmin(context.Context, *adminpb.CheckAdminRequest) (*adminpb.CheckAdminResponse, error) {
	return &adminpb.CheckAdminResponse{Admin: true}, nil
}

func (f *fakeQuotaAdmin) RequestAdmin(context.Context, *adminpb.RequestAdminRequest) (*adminpb.RequestAdminResponse, error) {
	return &adminpb.RequestAdminResponse{Token: "admin"}, nil
}

func (f *fakeQuotaAdmin) GetSpaceQuota(_ context.Context, req *adminpb.GetSpaceQuotaRequest) (*adminpb.SpaceQuota, error) {
	if req.Ref.GetName() != "cernbox" {
		return nil, status.Error(codes.NotFound, "no such space")
	}
	return &adminpb.SpaceQuota{Root: "/eos/project/c/cernbox", Quota: &provider.Quota{QuotaMaxBytes: 1e12, RemainingBytes: 6e11, QuotaMaxFiles: 1e6, RemainingFiles: 999_000}, PhysicalMaxBytes: 2e12, Form: "project", MountId: "eosproject-c"}, nil
}

func (f *fakeQuotaAdmin) SetSpaceQuota(_ context.Context, req *adminpb.SetSpaceQuotaRequest) (*adminpb.SpaceQuota, error) {
	f.mu.Lock()
	f.sets = append(f.sets, req)
	f.mu.Unlock()
	limit := req.GetQuota().GetQuotaMaxBytes()
	return &adminpb.SpaceQuota{Root: "/eos/project/c/cernbox", Quota: &provider.Quota{QuotaMaxBytes: limit, RemainingBytes: limit}, PhysicalMaxBytes: 2 * limit, Form: "project"}, nil
}

func (f *fakeQuotaAdmin) ListSpaceQuotas(req *adminpb.ListSpaceQuotasRequest, s adminpb.SpacesAdmin_ListSpaceQuotasServer) error {
	f.mu.Lock()
	f.min = req.MinBytes
	f.mu.Unlock()
	for _, root := range []string{"/eos/user/b/bob", "/eos/user/a/alice"} {
		if err := s.Send(&adminpb.SpaceQuota{Root: root, Quota: &provider.Quota{QuotaMaxBytes: 1e12, RemainingBytes: 1e12}, MountId: "eoshome-a"}); err != nil {
			return err
		}
	}
	return nil
}

func runQuota(t *testing.T, f *fakeQuotaAdmin, args ...string) (string, error) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(s, f)
	adminpb.RegisterSpacesAdminServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	global := []string{"--admin-host", lis.Addr().String(), "--insecure", "--token", "login", "--yes"}
	return runFake(t, clitest.New("alice"), append(global, args...)...)
}

// A space the user plane does not find is read with admin rights.
func TestSpaceQuotaGetFallsBackToTheAdminPlane(t *testing.T) {
	out, err := runQuota(t, &fakeQuotaAdmin{}, "space", "quota", "get", "cernbox")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	// What is used is max - remaining.
	for _, want := range []string{"/eos/project/c/cernbox", "400 GB", "1.0 TB", "2.0 TB", "1000/1000000", "project"} {
		if !strings.Contains(out, want) {
			t.Errorf("output has no %q:\n%s", want, out)
		}
	}
	if _, err := runQuota(t, &fakeQuotaAdmin{}, "space", "quota", "get", "nope"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("a space that does not exist: %v, want the not-found of the user plane", err)
	}
}

func TestSpaceQuotaSet(t *testing.T) {
	f := &fakeQuotaAdmin{}
	out, err := runQuota(t, f, "space", "quota", "set", "cernbox", "2T", "-o", "json")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(f.sets) != 1 || f.sets[0].GetQuota().GetQuotaMaxBytes() != 2e12 || f.sets[0].Ref.GetName() != "cernbox" || !slices.Equal(f.sets[0].GetUpdateMask().GetPaths(), []string{"quota_max_bytes"}) {
		t.Fatalf("sets = %v", f.sets)
	}
	if !strings.Contains(out, `"physicalMaxBytes": "4000000000000"`) {
		t.Errorf("output = %s", out)
	}

	if _, err := runQuota(t, f, "space", "quota", "set", "cernbox", "lots"); err == nil || !strings.Contains(err.Error(), "not a size") {
		t.Errorf("a bad size: %v", err)
	}
	if len(f.sets) != 1 {
		t.Errorf("a bad size set the quota")
	}
}

func TestSpaceQuotaList(t *testing.T) {
	f := &fakeQuotaAdmin{}
	out, err := runQuota(t, f, "space", "quota", "list", "--min", "10G")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if f.min != 1e10 {
		t.Errorf("min_bytes = %d, want 10G", f.min)
	}
	if a, b := strings.Index(out, "/eos/user/a/alice"), strings.Index(out, "/eos/user/b/bob"); a < 0 || b < a {
		t.Errorf("want the quotas sorted by root:\n%s", out)
	}
}
