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
	"strings"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func TestAuthenticateRejectsMalformedClientID(t *testing.T) {
	tests := []struct {
		name     string
		clientID string
	}{
		{name: "empty", clientID: ""},
		{name: "single label", clientID: "receiver"},
		{name: "scheme", clientID: "https://receiver.example"},
		{name: "ip", clientID: "192.0.2.10"},
		{name: "port", clientID: "receiver.example:443"},
		{name: "whitespace", clientID: "receiver.example "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &mockGW{
				share:     testShare("share-abc", "code123"),
				shareErr:  rpc.Code_CODE_OK,
				remoteErr: rpc.Code_CODE_OK,
			}
			stampGateway(gw)
			mgr := &manager{c: &config{}}

			user, scopes, err := mgr.Authenticate(context.Background(), tt.clientID, "code123")
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
			if gw.shareCalls != 0 || gw.acceptedCalls != 0 {
				t.Fatalf("calls: share=%d accepted=%d, want 0", gw.shareCalls, gw.acceptedCalls)
			}
			cred, ok := err.(errtypes.InvalidCredentials)
			if !ok || string(cred) != "invalid ocm client_id" {
				t.Fatalf("error: got %T %v, want InvalidCredentials invalid ocm client_id", err, err)
			}
			if tt.clientID != "" && strings.Contains(err.Error(), tt.clientID) {
				t.Fatalf("error echoed rejected input: %v", err)
			}
		})
	}
}

func TestAuthenticateBindsStoredRecipient(t *testing.T) {
	minter := newTestMinter(t)
	matched := applyShareMutators(
		testShare("share-alpha", "code-alpha"),
		mutateRecipient("Receiver-A.Example"),
		mutateCreator("sender-alpha.example"),
		mutateOwner("owner-alpha.example"),
	)
	got := mintAuthenticatedShare(t, minter, matched, "receiver-a.example")
	if got != "share-alpha" {
		t.Fatalf("client_id: got %q, want share-alpha", got)
	}

	tests := []struct {
		name     string
		mutate   func(*ocm.Share)
		clientID string
	}{
		{name: "different valid receiver", clientID: "receiver-b.example"},
		{name: "sender domain is not the recipient", clientID: "sender-alpha.example"},
		{name: "owner domain is not the recipient", clientID: "owner-alpha.example"},
		{
			name:     "empty stored idp",
			mutate:   mutateEmptyRecipient,
			clientID: "receiver-a.example",
		},
		{
			name:     "invalid stored idp",
			mutate:   mutateInvalidRecipient,
			clientID: "receiver-a.example",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			share := applyShareMutators(
				testShare("share-alpha", "code-alpha"),
				mutateRecipient("receiver-a.example"),
				mutateCreator("sender-alpha.example"),
				mutateOwner("owner-alpha.example"),
			)
			if tt.mutate != nil {
				tt.mutate(share)
			}
			gw := &mockGW{share: share, shareErr: rpc.Code_CODE_OK, remoteErr: rpc.Code_CODE_OK}
			stampGateway(gw)
			mgr := &manager{c: &config{}}

			user, scopes, err := mgr.Authenticate(context.Background(), tt.clientID, share.Token)
			if user != nil || scopes != nil {
				t.Fatalf("auth result: user=%v scopes=%v, want no token inputs", user, scopes)
			}
			if gw.shareCalls != 1 || gw.acceptedCalls != 0 {
				t.Fatalf("calls: share=%d accepted=%d", gw.shareCalls, gw.acceptedCalls)
			}
			cred, ok := err.(errtypes.InvalidCredentials)
			if !ok || string(cred) != "ocm share receiver does not match client_id" {
				t.Fatalf("error: got %T %v", err, err)
			}
			if strings.Contains(err.Error(), tt.clientID) {
				t.Fatalf("error echoed rejected input: %v", err)
			}
		})
	}
}
