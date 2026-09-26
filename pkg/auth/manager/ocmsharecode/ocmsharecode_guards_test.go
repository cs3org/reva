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

package ocmsharecode

import (
	"context"
	"errors"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestAuthenticateResponseAndGranteeGuards(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*ocm.Share)
		nilResponse  bool
		nilStatus    bool
		gatewayErr   error
		wantClass    string
		wantMessage  string
		wantShareHit int
	}{
		{
			name:         "gateway error before nil response",
			gatewayErr:   errors.New("gateway down"),
			nilResponse:  true,
			wantClass:    "plain",
			wantMessage:  "gateway down",
			wantShareHit: 1,
		},
		{
			name:         "nil response",
			nilResponse:  true,
			wantClass:    "internal",
			wantMessage:  "missing ocm share response",
			wantShareHit: 1,
		},
		{
			name:         "nil status",
			nilStatus:    true,
			wantClass:    "internal",
			wantMessage:  "missing ocm share response",
			wantShareHit: 1,
		},
		{
			name:         "nil grantee",
			mutate:       mutateNilGrantee,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing grantee",
			wantShareHit: 1,
		},
		{
			name:         "absent grantee id",
			mutate:       mutateAbsentGranteeID,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing grantee",
			wantShareHit: 1,
		},
		{
			name:         "group grantee",
			mutate:       mutateGroupGrantee,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing grantee",
			wantShareHit: 1,
		},
		{
			name:         "nil user payload",
			mutate:       mutateNilUserPayload,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing grantee",
			wantShareHit: 1,
		},
		{
			name:         "missing opaque id before grantee",
			mutate:       mutateMissingOpaqueAndGrantee,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing provider id",
			wantShareHit: 1,
		},
		{
			name:         "blank opaque id before recipient check",
			mutate:       mutateBlankOpaqueID,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing provider id",
			wantShareHit: 1,
		},
		{
			name:         "nil share id",
			mutate:       mutateMissingOpaqueID,
			wantClass:    "credentials",
			wantMessage:  "ocm share is missing provider id",
			wantShareHit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			share := testShare("share-abc", "code123")
			if tt.mutate != nil {
				tt.mutate(share)
			}
			gw := &mockGW{
				share:       share,
				shareErr:    rpc.Code_CODE_OK,
				remoteErr:   rpc.Code_CODE_OK,
				shareRPCErr: tt.gatewayErr,
				nilResponse: tt.nilResponse,
				nilStatus:   tt.nilStatus,
			}
			stampGateway(gw)
			mgr := &manager{c: &config{}}

			user, scopes, err := mgr.Authenticate(context.Background(), "remote.example.com", "code123")
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
			if gw.shareCalls != tt.wantShareHit || gw.acceptedCalls != 0 {
				t.Fatalf("calls: share=%d accepted=%d, want share=%d accepted=0", gw.shareCalls, gw.acceptedCalls, tt.wantShareHit)
			}
			assertAuthError(t, err, tt.wantClass, tt.wantMessage)
		})
	}
}

func TestAuthenticateAcceptedUserResponseGuards(t *testing.T) {
	tests := []struct {
		name                string
		nilAcceptedResponse bool
		nilAcceptedStatus   bool
	}{
		{
			name:                "nil accepted user response",
			nilAcceptedResponse: true,
		},
		{
			name:              "nil accepted user status",
			nilAcceptedStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			share := testShare("share-abc", "code123")
			gw := &mockGW{
				share:               share,
				shareErr:            rpc.Code_CODE_OK,
				remoteErr:           rpc.Code_CODE_OK,
				nilAcceptedResponse: tt.nilAcceptedResponse,
				nilAcceptedStatus:   tt.nilAcceptedStatus,
			}
			stampGateway(gw)
			mgr := &manager{c: &config{}}

			user, scopes, err := mgr.Authenticate(context.Background(), "remote.example.com", "code123")
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
			if gw.shareCalls != 1 || gw.acceptedCalls != 1 {
				t.Fatalf("calls: share=%d accepted=%d, want share=1 accepted=1", gw.shareCalls, gw.acceptedCalls)
			}
			if gw.lastToken != "code123" {
				t.Fatalf("lookup key: got %q, want code123", gw.lastToken)
			}
			assertAuthError(t, err, "internal", "missing accepted user response")
		})
	}
}

func assertAuthError(t *testing.T, err error, class, message string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	switch class {
	case "internal":
		got, ok := err.(errtypes.InternalError)
		if !ok || string(got) != message {
			t.Fatalf("error: got %T %v, want InternalError %q", err, err, message)
		}
	case "credentials":
		got, ok := err.(errtypes.InvalidCredentials)
		if !ok || string(got) != message {
			t.Fatalf("error: got %T %v, want InvalidCredentials %q", err, err, message)
		}
	case "plain":
		if _, ok := err.(errtypes.InternalError); ok {
			t.Fatalf("gateway error was replaced: %v", err)
		}
		if _, ok := err.(errtypes.InvalidCredentials); ok {
			t.Fatalf("gateway error was replaced: %v", err)
		}
		if err.Error() != message {
			t.Fatalf("error: got %v, want %q", err, message)
		}
	default:
		t.Fatalf("unknown class %q", class)
	}
}
