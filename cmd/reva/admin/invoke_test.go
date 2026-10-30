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
	"sync"
	"testing"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAdmin is an Admin API with one service, svc, on two nodes.
type fakeAdmin struct {
	adminpb.UnimplementedAdminAPIServer
	specs   []*adminpb.InvocationSpec
	listErr error
	results []*adminpb.NodeResult
	stream  []*adminpb.InvokeStreamResponse

	mu      sync.Mutex
	invoked []*adminpb.InvokeRequest
}

func (f *fakeAdmin) RequestAdmin(context.Context, *adminpb.RequestAdminRequest) (*adminpb.RequestAdminResponse, error) {
	return &adminpb.RequestAdminResponse{Token: "admin"}, nil
}

func (f *fakeAdmin) ListServices(context.Context, *adminpb.ListServicesRequest) (*adminpb.ListServicesResponse, error) {
	return &adminpb.ListServicesResponse{Services: []*adminpb.Service{
		{Name: "svc", Nodes: []*adminpb.Node{{Id: "a:1/svc", State: "ready"}, {Id: "b:1/svc", State: "ready"}}},
		{Name: "gateway", Nodes: []*adminpb.Node{{Id: "a:2/gateway", State: "ready"}}},
	}}, nil
}

func (f *fakeAdmin) ListInvocations(context.Context, *adminpb.ListInvocationsRequest) (*adminpb.ListInvocationsResponse, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &adminpb.ListInvocationsResponse{Invocations: f.specs}, nil
}

func (f *fakeAdmin) Invoke(_ context.Context, req *adminpb.InvokeRequest) (*adminpb.InvokeResponse, error) {
	f.mu.Lock()
	f.invoked = append(f.invoked, req)
	f.mu.Unlock()
	return &adminpb.InvokeResponse{Results: f.results}, nil
}

func (f *fakeAdmin) InvokeStream(req *adminpb.InvokeRequest, s adminpb.AdminAPI_InvokeStreamServer) error {
	f.mu.Lock()
	f.invoked = append(f.invoked, req)
	f.mu.Unlock()
	for _, m := range f.stream {
		if err := s.Send(m); err != nil {
			return err
		}
	}
	return nil
}

func run(t *testing.T, f *fakeAdmin, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("NO_COLOR", "1")
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	adminpb.RegisterAdminAPIServer(s, f)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	r := &cli.Runtime{}
	root := &cobra.Command{
		Use: "reva", SilenceErrors: true, SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return r.Load(cmd.Flags()) },
	}
	r.AddFlags(root.PersistentFlags())
	root.AddCommand(Commands(r)...)
	global := []string{"--admin-host", lis.Addr().String(), "--insecure", "--token", "login", "--yes"}
	if len(args) > 0 && args[0] == cobra.ShellCompRequestCmd {
		args = append(append([]string{args[0]}, global...), args[1:]...)
	} else {
		args = append(global, args...)
	}
	root.SetArgs(args)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	err = root.Execute()
	r.Close()
	return out.String(), errOut.String(), err
}

var opSpec = &adminpb.InvocationSpec{
	Name: "op", Description: "an operation", Kind: "readonly",
	Args: []*adminpb.ArgSpec{
		{Name: "ids", Description: "the ids", Required: true, Type: "string", Repeated: true},
		{Name: "level", Description: "the level", Type: "string", Default: "info", Enum: []string{"info", "warn"}},
		{Name: "all", Type: "bool"},
	},
}

func TestInvokeChecksTheArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		err  string
	}{
		{[]string{"level=info"}, "needs the argument ids"},
		{[]string{"ids=a", "typo=1"}, `no argument "typo"`},
		{[]string{"ids=a", "level=debug"}, `"debug" is not one of info, warn`},
		{[]string{"ids"}, "not key=value"},
		{[]string{"ids=a,b", "level=warn"}, ""},
	} {
		f := &fakeAdmin{specs: []*adminpb.InvocationSpec{opSpec}, results: []*adminpb.NodeResult{{Node: "a:1/svc", ResultJson: `{}`}}}
		_, _, err := run(t, f, append([]string{"invoke", "svc", "op"}, tc.args...)...)
		if tc.err == "" {
			if err != nil || len(f.invoked) != 1 {
				t.Errorf("%v: err = %v, %d calls: want one call", tc.args, err, len(f.invoked))
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.err) || len(f.invoked) != 0 {
			t.Errorf("%v: err = %v, %d calls: want %q and no call", tc.args, err, len(f.invoked), tc.err)
		}
	}
}

// Without a full catalogue, the server decides.
func TestInvokeSendsWithoutTheCatalogue(t *testing.T) {
	for name, f := range map[string]*fakeAdmin{
		"no catalogue":       {listErr: status.Error(codes.Unavailable, "down")},
		"names only":         {specs: []*adminpb.InvocationSpec{{Name: "op"}}},
		"no such invocation": {specs: []*adminpb.InvocationSpec{opSpec}},
	} {
		invocation := "op"
		if name == "no such invocation" {
			invocation = "other"
		}
		if _, _, err := run(t, f, "invoke", "svc", invocation, "typo=1"); err != nil || len(f.invoked) != 1 {
			t.Errorf("%s: err = %v, %d calls: want one call", name, err, len(f.invoked))
		}
	}
}

func TestInvokeCompletion(t *testing.T) {
	f := &fakeAdmin{specs: []*adminpb.InvocationSpec{opSpec, {Name: "version", Description: "the version", Kind: "readonly"}}}
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"invoke", ""}, []string{"svc", "gateway"}},
		{[]string{"invoke", "svc", ""}, []string{"op\tan operation", "version\tthe version"}},
		{[]string{"invoke", "svc", "op", ""}, []string{"ids=\tthe ids", "level=\tthe level", "all="}},
		{[]string{"invoke", "svc", "op", "ids=a", ""}, []string{"level=\tthe level", "all="}},
		{[]string{"invoke", "svc", "op", "level="}, []string{"level=info", "level=warn"}},
		{[]string{"invoke", "svc", "op", "all="}, []string{"all=true", "all=false"}},
		{[]string{"invocation", "get", "svc", ""}, []string{"op\tan operation", "version\tthe version"}},
		{[]string{"invocation", "get", "svc", "op", ""}, nil},
	} {
		out, _, err := run(t, f, append([]string{cobra.ShellCompRequestCmd}, tc.args...)...)
		if err != nil {
			t.Fatal(err)
		}
		// The last line is the directive of cobra.
		lines := strings.Split(strings.TrimSpace(out), "\n")
		got := lines[:len(lines)-1]
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%v: got %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestInvocationGetShowsTheArguments(t *testing.T) {
	f := &fakeAdmin{specs: []*adminpb.InvocationSpec{opSpec}}
	for _, args := range [][]string{{"invocation", "get", "svc", "op"}, {"invocation", "list", "svc", "op"}} {
		out, _, err := run(t, f, args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"kind: readonly", "list of string", "info, warn", "the level"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v: no %q in\n%s", args, want, out)
			}
		}
	}

	out, _, err := run(t, f, "-o", "json", "invocation", "get", "svc", "op")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"enum": [`) || !strings.Contains(out, `"repeated": true`) {
		t.Errorf("json: %s", out)
	}

	out, _, err = run(t, f, "invocation", "list", "svc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ids [level] [all]") {
		t.Errorf("list: no compact arguments in\n%s", out)
	}
}
