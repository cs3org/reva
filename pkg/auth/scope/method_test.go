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

package scope

import (
	"context"
	"slices"
	"strings"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	_ "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	_ "github.com/cs3org/reva/v3/pkg/admin/adminpb"
	_ "github.com/cs3org/reva/v3/pkg/control/controlpb"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// methods returns the full gRPC method names of every service in the proto
// packages that match.
func methods(match func(pkg string) bool) []string {
	var out []string
	protoregistry.GlobalFiles.RangeFiles(func(f protoreflect.FileDescriptor) bool {
		if !match(string(f.Package())) {
			return true
		}
		for i := range f.Services().Len() {
			sd := f.Services().Get(i)
			for j := range sd.Methods().Len() {
				out = append(out, "/"+string(sd.FullName())+"/"+string(sd.Methods().Get(j).Name()))
			}
		}
		return true
	})
	return out
}

func mustScopes(t *testing.T, add ...func(map[string]*authpb.Scope) (map[string]*authpb.Scope, error)) map[string]*authpb.Scope {
	t.Helper()
	var scopes map[string]*authpb.Scope
	for _, f := range add {
		var err error
		if scopes, err = f(scopes); err != nil {
			t.Fatal(err)
		}
	}
	return scopes
}

// TestEveryAdminMethodNeedsTheAdminScope walks every service of the admin and
// control packages, so that a new typed admin service is covered without a
// change to the scopes.
func TestEveryAdminMethodNeedsTheAdminScope(t *testing.T) {
	ctx := context.Background()
	lightweight := func(s map[string]*authpb.Scope) (map[string]*authpb.Scope, error) {
		return AddLightweightAccountScope(authpb.Role_ROLE_OWNER, s)
	}
	admin := mustScopes(t, AddAdminScope)
	others := map[string]map[string]*authpb.Scope{
		"user":           mustScopes(t, AddOwnerScope),
		"lightweight":    mustScopes(t, lightweight),
		"machine":        mustScopes(t, AddMachineScope),
		"machine + user": mustScopes(t, AddOwnerScope, AddMachineScope),
	}

	services := protoServices()
	for _, svc := range []string{"reva.admin.v1beta1.AdminAPI", "reva.admin.v1beta1.SpacesAdmin",
		"reva.admin.v1beta1.SharesAdmin", "reva.admin.v1beta1.StorageAdmin", "reva.control.v1beta1.Control"} {
		if !slices.Contains(services, svc) {
			t.Fatalf("service %s is not in the proto registry", svc)
		}
	}

	ms := methods(func(pkg string) bool { return slices.Contains(adminPackages, pkg) })
	for _, m := range ms {
		stepUp := slices.Contains(stepUpMethods, m)
		if got, _ := VerifyScope(ctx, admin, MethodResource(m)); got == stepUp {
			t.Errorf("admin token, %s: got %v, want %v", m, got, !stepUp)
		}
		for name, scopes := range others {
			want := stepUp && name != "lightweight"
			if got, _ := VerifyScope(ctx, scopes, MethodResource(m)); got != want {
				t.Errorf("%s token, %s: got %v, want %v", name, m, got, want)
			}
		}
	}
}

// protoServices lists the full names of every registered service, also the
// services that have no methods yet.
func protoServices() []string {
	var out []string
	protoregistry.GlobalFiles.RangeFiles(func(f protoreflect.FileDescriptor) bool {
		for i := range f.Services().Len() {
			out = append(out, string(f.Services().Get(i).FullName()))
		}
		return true
	})
	return out
}

// TestAdminTokenSatisfiesNoCS3Request walks every registered CS3 request
// message and CS3 method.
func TestAdminTokenSatisfiesNoCS3Request(t *testing.T) {
	ctx := context.Background()
	admin := mustScopes(t, AddAdminScope)

	var n int
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		name := string(mt.Descriptor().FullName())
		if !strings.HasPrefix(name, "cs3.") || !strings.HasSuffix(name, "Request") {
			return true
		}
		n++
		if got, _ := VerifyScope(ctx, admin, mt.New().Interface()); got {
			t.Errorf("admin token satisfies %s", name)
		}
		return true
	})
	for _, m := range methods(func(pkg string) bool { return strings.HasPrefix(pkg, "cs3.") }) {
		n++
		if got, _ := VerifyScope(ctx, admin, MethodResource(m)); got {
			t.Errorf("admin token satisfies %s", m)
		}
	}
	if n == 0 {
		t.Fatal("no CS3 request in the proto registry")
	}
}
