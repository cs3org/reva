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
	"errors"
	"fmt"
	"strings"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeGate struct {
	admin      bool
	confirmErr error
	canAdmin   int
	adminCtx   int
	confirms   int
	warnings   []string
}

func (g *fakeGate) CanAdmin(context.Context) bool { g.canAdmin++; return g.admin }
func (g *fakeGate) AdminContext(ctx context.Context) (context.Context, error) {
	g.adminCtx++
	return ctx, nil
}
func (g *fakeGate) Confirm(string) error { g.confirms++; return g.confirmErr }
func (g *fakeGate) Warnf(format string, args ...any) {
	g.warnings = append(g.warnings, fmt.Sprintf(format, args...))
}

type share struct{ id, owner string }

var shares = map[string]share{"4711": {"4711", "bob"}}

func userUpdate(err error) func(context.Context) (*share, error) {
	return func(context.Context) (*share, error) { return nil, err }
}

func adminLookup(id string, lookups *int) func(context.Context) (*share, error) {
	return func(ctx context.Context) (*share, error) {
		*lookups++
		s, ok := shares[id]
		if !ok {
			return nil, status.Error(codes.NotFound, "share not found")
		}
		return &s, nil
	}
}

func TestOnNotFound(t *testing.T) {
	notFound := Check(&rpc.Status{Code: rpc.Code_CODE_NOT_FOUND, Message: "share not found"})
	grpcNotFound := status.Error(codes.Unknown, "gateway: error calling UpdateShare: share not found")
	authErr := status.Error(codes.Unauthenticated, "token expired")

	for _, tc := range []struct {
		name        string
		id          string
		userErr     error
		admin       bool
		confirmErr  error
		wantErr     error
		wantAdmin   bool
		wantLookup  bool
		wantCheck   bool
		wantConfirm bool
	}{
		{name: "admin on the share of another user, body status", id: "4711", userErr: notFound, admin: true,
			wantAdmin: true, wantLookup: true, wantCheck: true, wantConfirm: true},
		{name: "admin on the share of another user, grpc status", id: "4711", userErr: grpcNotFound, admin: true,
			wantAdmin: true, wantLookup: true, wantCheck: true, wantConfirm: true},
		{name: "admin with a mistyped id", id: "4712", userErr: notFound, admin: true,
			wantErr: notFound, wantLookup: true, wantCheck: true},
		{name: "non-admin", id: "4711", userErr: notFound,
			wantErr: notFound, wantCheck: true},
		{name: "grpc auth error", id: "4711", userErr: authErr, admin: true,
			wantErr: authErr},
		{name: "mutation without a terminal", id: "4711", userErr: notFound, admin: true, confirmErr: errors.New("refused without a terminal"),
			wantLookup: true, wantCheck: true, wantConfirm: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGate{admin: tc.admin, confirmErr: tc.confirmErr}
			var lookups int
			var adminCalls int
			got, err := OnNotFound(context.Background(), Fallback{Gate: g, What: "share " + tc.id},
				userUpdate(tc.userErr), adminLookup(tc.id, &lookups),
				func(ctx context.Context, s *share) (*share, error) {
					adminCalls++
					if !InFallback(ctx) {
						t.Error("the admin call does not know that it runs in a fallback")
					}
					return s, nil
				})
			switch {
			case tc.confirmErr != nil:
				if !errors.Is(err, tc.confirmErr) {
					t.Fatalf("err = %v, want %v", err, tc.confirmErr)
				}
			case tc.wantErr != nil:
				// The original error, unchanged: a non-admin learns nothing.
				if err != tc.wantErr {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case err != nil || got == nil || got.owner != "bob":
				t.Fatalf("got %v, %v, want the share of bob", got, err)
			}
			if (adminCalls == 1) != tc.wantAdmin {
				t.Errorf("admin calls = %d, want %v", adminCalls, tc.wantAdmin)
			}
			if (lookups == 1) != tc.wantLookup || (g.adminCtx == 1) != tc.wantLookup {
				t.Errorf("lookups = %d, admin contexts = %d, want %v", lookups, g.adminCtx, tc.wantLookup)
			}
			if (g.canAdmin == 1) != tc.wantCheck {
				t.Errorf("CanAdmin calls = %d, want %v", g.canAdmin, tc.wantCheck)
			}
			if (g.confirms == 1) != tc.wantConfirm {
				t.Errorf("confirmations = %d, want %v", g.confirms, tc.wantConfirm)
			}
			if tc.wantConfirm && (len(g.warnings) != 1 || g.warnings[0] != "share 4711 is not yours: changing it with admin rights") {
				t.Errorf("warnings = %q", g.warnings)
			}
		})
	}
}

func TestOnNotFoundGetUsesTheLookup(t *testing.T) {
	g := &fakeGate{admin: true}
	var lookups int
	got, err := OnNotFound(context.Background(), Fallback{Gate: g, What: "share 4711"},
		userUpdate(Check(&rpc.Status{Code: rpc.Code_CODE_NOT_FOUND})), adminLookup("4711", &lookups), nil)
	if err != nil || got.owner != "bob" {
		t.Fatalf("got %v, %v", got, err)
	}
	if g.confirms != 0 || len(g.warnings) != 1 || !strings.Contains(g.warnings[0], "showing it") {
		t.Errorf("a read must not ask, and must tell: confirms=%d warnings=%q", g.confirms, g.warnings)
	}
}

func TestOnNotFoundKeepsTheErrorWhenTheAdminRPCIsMissing(t *testing.T) {
	notFound := Check(&rpc.Status{Code: rpc.Code_CODE_NOT_FOUND})
	_, err := OnNotFound(context.Background(), Fallback{Gate: &fakeGate{admin: true}, What: "share 4711"},
		userUpdate(notFound),
		func(context.Context) (*share, error) { return nil, ErrNotAvailable },
		func(context.Context, *share) (*share, error) {
			t.Fatal("no admin call without a lookup")
			return nil, nil
		})
	if !IsNotFound(err) || !strings.Contains(err.Error(), ErrNotAvailable.Error()) {
		t.Fatalf("err = %v, want the not-found with a note", err)
	}
}
