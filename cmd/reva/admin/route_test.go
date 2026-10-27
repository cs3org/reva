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
	"bytes"
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	registrypb "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

type fakeStorage struct {
	adminpb.UnimplementedStorageAdminServer
	stale    []*adminpb.StaleInstance
	added    []*adminpb.AddRouteRequest
	resolved []*adminpb.ResolvePathRequest
}

func (f *fakeStorage) ListRoutes(context.Context, *adminpb.ListRoutesRequest) (*adminpb.ListRoutesResponse, error) {
	return &adminpb.ListRoutesResponse{Routes: []*registrypb.ProviderInfo{{ProviderPath: "/eos/user/a", ProviderId: "eoshome-i01"}}}, nil
}

func (f *fakeStorage) ResolvePath(_ context.Context, req *adminpb.ResolvePathRequest) (*adminpb.ResolvePathResponse, error) {
	f.resolved = append(f.resolved, req)
	return &adminpb.ResolvePathResponse{Path: req.GetRef().GetPath(), Providers: []*adminpb.ResolvedProvider{{
		Provider:      &registrypb.ProviderInfo{ProviderId: "eoshome-i01", ProviderPath: "/eos/user/a", Address: "lb:9000"},
		ProviderNodes: []string{"sp1:9001/storageprovider", "sp2:9001/storageprovider"},
	}}}, nil
}

func (f *fakeStorage) AddRoute(_ context.Context, req *adminpb.AddRouteRequest) (*adminpb.AddRouteResponse, error) {
	f.added = append(f.added, req)
	return &adminpb.AddRouteResponse{Route: &registrypb.ProviderInfo{ProviderPath: req.GetRoute().GetProviderPath(), ProviderId: req.GetRoute().GetProviderId()}, Stale: f.stale}, nil
}

func runRoute(t *testing.T, f *fakeStorage, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(s, &fakeAdmin{})
	adminpb.RegisterStorageAdminServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	r := &cli.Runtime{}
	root := &cobra.Command{
		Use: "reva", SilenceErrors: true, SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return r.Load(cmd.Flags()) },
	}
	r.AddFlags(root.PersistentFlags())
	root.AddCommand(RouteCommand(r))
	root.SetArgs(append([]string{"--admin-host", lis.Addr().String(), "--insecure", "--token", "login", "--yes"}, args...))
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err = root.Execute()
	r.Close()
	return out.String(), errOut.String(), err
}

func TestRouteAdd(t *testing.T) {
	f := &fakeStorage{}
	out, _, err := runRoute(t, f, "route", "add", "/eos/user/z", "eoshome-i05")
	if err != nil || !strings.Contains(out, "/eos/user/z is routed to eoshome-i05") || len(f.added) != 1 {
		t.Fatalf("route add: %v, %q, %v", err, out, f.added)
	}
	if r := f.added[0].GetRoute(); r.GetProviderPath() != "/eos/user/z" || r.GetProviderId() != "eoshome-i05" || r.GetAddress() != "" {
		t.Errorf("route add sent %v", r)
	}
}

func TestRouteAddWithAStaleInstanceFails(t *testing.T) {
	f := &fakeStorage{stale: []*adminpb.StaleInstance{{Node: "reg3:9999/storageregistry", Status: &rpc.Status{Code: rpc.Code_CODE_UNAVAILABLE, Message: "db unreachable"}}}}
	out, errOut, err := runRoute(t, f, "route", "add", "/eos/user/z", "eoshome-i05")
	if err == nil || !strings.Contains(err.Error(), "1 storage registry instances did not reload") {
		t.Fatalf("err = %v, want the stale instances", err)
	}
	if !strings.Contains(out, "routed to eoshome-i05") || !strings.Contains(errOut, "stale: reg3:9999/storageregistry: db unreachable (CODE_UNAVAILABLE)") {
		t.Errorf("stdout %q, stderr %q", out, errOut)
	}
}

func TestRouteListAndResolve(t *testing.T) {
	out, _, err := runRoute(t, &fakeStorage{}, "route", "list")
	if err != nil || !strings.Contains(out, "/eos/user/a") || !strings.Contains(out, "eoshome-i01") {
		t.Fatalf("route list: %v\n%s", err, out)
	}
	f := &fakeStorage{}
	out, _, err = runRoute(t, f, "route", "resolve", "/eos/user/a/alice")
	if err != nil || !strings.Contains(out, "sp1:9001/storageprovider,sp2:9001/storageprovider") || !strings.Contains(out, "lb:9000") {
		t.Fatalf("route resolve: %v\n%s", err, out)
	}
	if len(f.resolved) != 1 || f.resolved[0].GetRef().GetPath() != "/eos/user/a/alice" || f.resolved[0].GetRef().GetResourceId() != nil {
		t.Errorf("route resolve sent %v, want a path reference", f.resolved)
	}
}
