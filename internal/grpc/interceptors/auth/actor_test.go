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

package auth

import (
	"bytes"
	"context"
	"strings"
	"testing"

	registry "github.com/cs3org/go-cs3apis/cs3/storage/registry/v1beta1"
	appctxinterceptor "github.com/cs3org/reva/v3/internal/grpc/interceptors/appctx"
	loginterceptor "github.com/cs3org/reva/v3/internal/grpc/interceptors/log"
	"github.com/cs3org/reva/v3/pkg/admin"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	"github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

func TestLogsShowTheActor(t *testing.T) {
	interceptor, login, u := setup(t)
	_ = userGroupsCache.Set(u.Id.OpaqueId, []string{"physicists"})

	mgr, err := jwt.New(map[string]any{"secret": testSecret})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := scope.AddOwnerScope(nil)
	if err != nil {
		t.Fatal(err)
	}
	impersonated, err := mgr.MintToken(context.Background(), u, scope.AddActorScope("gdelmont", owner))
	if err != nil {
		t.Fatal(err)
	}

	run := func(tkn string) []string {
		var buf bytes.Buffer
		chain := grpc_middleware.ChainUnaryServer(
			appctxinterceptor.NewUnary(zerolog.New(&buf).Level(zerolog.DebugLevel), nil),
			loginterceptor.NewUnary(),
			interceptor,
		)
		ctx := appctx.ContextSetToken(context.Background(), tkn)
		_, err := chain(ctx, &registry.GetStorageProvidersRequest{}, &grpc.UnaryServerInfo{
			FullMethod: "/cs3.storage.registry.v1beta1.RegistryAPI/GetStorageProviders",
		}, func(ctx context.Context, _ any) (any, error) {
			admin.Audit(ctx, admin.AuditEvent{Action: "test", Actor: "einstein", Granted: true})
			return nil, nil
		})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		return strings.Split(strings.TrimSpace(buf.String()), "\n")
	}

	lines := run(impersonated)
	for _, msg := range []string{`"message":"unary"`, `"message":"admin audit"`} {
		var found bool
		for _, l := range lines {
			if strings.Contains(l, msg) {
				found = true
				if !strings.Contains(l, `"act":"gdelmont"`) {
					t.Errorf("impersonated token: %s has no act: %s", msg, l)
				}
			}
		}
		if !found {
			t.Errorf("no line %s in %v", msg, lines)
		}
	}

	for _, l := range run(login) {
		if strings.Contains(l, `"act"`) {
			t.Errorf("login token: a line shows an actor: %s", l)
		}
	}
}
