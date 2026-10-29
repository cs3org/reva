// Copyright 2018-2024 CERN
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

package dataprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
)

// The data provider writes to the storage without the gRPC interceptor, so an
// app password must be held to its permissions here.
func TestAppAuthAllowed(t *testing.T) {
	appAuth := func(perms ...string) map[string]*authpb.Scope {
		s, err := scope.AddAppAuthScope(&scope.AppAuthScope{Client: scope.ClientWebDAV, Permissions: perms}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, readWrite := appAuth(scope.ReadOnlyPermissions()...), appAuth(scope.ReadWritePermissions()...)
	const file = "/eos/user/e/einstein/a.txt"

	cases := []struct {
		name     string
		scopes   map[string]*authpb.Scope
		method   string
		protocol string
		path     string
		want     bool
	}{
		{"read-only download", readOnly, http.MethodGet, "simple", file, true},
		{"read-only head", readOnly, http.MethodHead, "simple", file, true},
		{"read-only upload", readOnly, http.MethodPut, "simple", file, false},
		{"read-write upload", readWrite, http.MethodPut, "simple", file, true},
		{"read-only upload to a space", readOnly, http.MethodPut, "spaces", "/eoshome-e$1!2/a.txt", false},
		{"read-write upload to a space", readWrite, http.MethodPut, "spaces", "/eoshome-e$1!2/a.txt", true},
		{"malformed space", readWrite, http.MethodPut, "spaces", "/nospace/a.txt", false},
		{"owner upload", owner, http.MethodPut, "simple", file, true},
		{"no scopes", nil, http.MethodPut, "simple", file, true},
	}
	for _, c := range cases {
		ctx := context.Background()
		if c.scopes != nil {
			ctx = appctx.ContextSetScopes(ctx, c.scopes)
		}
		r := httptest.NewRequest(c.method, "/data/"+c.protocol+c.path, nil).WithContext(ctx)
		if got := appAuthAllowed(r, c.protocol, c.path); got != c.want {
			t.Errorf("%s: allowed = %v, want %v", c.name, got, c.want)
		}
	}
}
