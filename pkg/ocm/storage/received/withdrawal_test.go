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

package ocm

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// switchingGateway serves the current share; a test can withdraw it.
type switchingGateway struct {
	gateway.GatewayAPIClient
	mu    sync.Mutex
	share *ocmpb.ReceivedShare
	// next, when set, replaces the share once after lookups were served.
	after int
	next  *ocmpb.ReceivedShare
	seen  int
	// onSwitch runs when the replacement share is served the first time.
	onSwitch func()
}

func (g *switchingGateway) GetReceivedOCMShare(_ context.Context, _ *ocmpb.GetReceivedOCMShareRequest, _ ...grpc.CallOption) (*ocmpb.GetReceivedOCMShareResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seen++
	s := g.share
	if g.next != nil && g.seen > g.after {
		s = g.next
		if g.onSwitch != nil {
			g.onSwitch()
			g.onSwitch = nil
		}
	}
	return &ocmpb.GetReceivedOCMShareResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, Share: s}, nil
}

func (g *switchingGateway) lookups() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.seen
}

func (g *switchingGateway) withdraw() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.share = withdrawnCopy(g.share)
}

func withdrawnCopy(s *ocmpb.ReceivedShare) *ocmpb.ReceivedShare {
	w := proto.Clone(s).(*ocmpb.ReceivedShare)
	w.State = ocmpb.ShareState_SHARE_STATE_INVALID
	w.Hidden = true
	return w
}

func requireLocalDenial(t *testing.T, op string, err error) {
	t.Helper()
	var denied errtypes.PermissionDenied
	if !errors.As(err, &denied) {
		t.Fatalf("%s: %v, want a local permission denial", op, err)
	}
	if isWebDAV401(err) {
		t.Fatalf("%s: a withdrawn share looks like a remote 401", op)
	}
}

// After withdrawal no operation reaches the sender, whatever was cached.
func TestWithdrawnShareAdmitsNoOperation(t *testing.T) {
	ref := &provider.Reference{Path: "/share-abc"}
	moveTo := &provider.Reference{Path: "/share-abc/renamed"}
	modes := map[string]*scriptedSender{
		"exchange cache":      {},
		"legacy bearer cache": {capabilities: []string{"invites"}},
		"legacy basic cache":  {capabilities: []string{"invites"}, rejectBearerSecret: true},
	}
	for name, script := range modes {
		t.Run(name, func(t *testing.T) {
			srv := startScriptedSender(t, script)
			share := scriptedShare(srv, false)
			gw := &switchingGateway{share: share}
			stampGateway(gw)
			d := newTestReceivedDriver(t)
			ctx, _ := loggedContext(t)

			// Seed the client and header caches with a working access.
			if _, err := d.GetMD(ctx, ref, nil); err != nil {
				t.Fatalf("seed: %v", err)
			}
			if _, ok := d.cachedShareClient(share.GetId().GetOpaqueId()); !ok {
				t.Fatal("no cached client was seeded")
			}
			disco, tokens, auths := script.snapshot()

			gw.withdraw()
			ops := map[string]func() error{
				"stat":     func() error { _, err := d.GetMD(ctx, ref, nil); return err },
				"list":     func() error { _, err := d.ListFolder(ctx, ref, nil); return err },
				"download": func() error { _, err := d.Download(ctx, ref, nil); return err },
				"upload": func() error {
					return d.Upload(ctx, ref, io.NopCloser(strings.NewReader("data")), nil)
				},
				"mkdir":  func() error { return d.CreateDir(ctx, ref) },
				"delete": func() error { return d.Delete(ctx, ref) },
				"touch":  func() error { return d.TouchFile(ctx, ref) },
				"move":   func() error { return d.Move(ctx, ref, moveTo) },
			}
			for op, fn := range ops {
				requireLocalDenial(t, op, fn())
			}
			d2, t2, a2 := script.snapshot()
			if d2 != disco || t2 != tokens || len(a2) != len(auths) {
				t.Fatalf("outbound after withdrawal: discovery %d->%d tokens %d->%d access %d->%d", disco, d2, tokens, t2, len(auths), len(a2))
			}
		})
	}
}

func TestWithdrawnStrictShareExchangesNothing(t *testing.T) {
	script := &scriptedSender{}
	srv := startScriptedSender(t, script)
	share := withdrawnCopy(scriptedShare(srv, true))
	stampGateway(&switchingGateway{share: share})
	d := newTestReceivedDriver(t)
	ctx, _ := loggedContext(t)
	_, err := d.GetMD(ctx, &provider.Reference{Path: "/share-abc"}, nil)
	requireLocalDenial(t, "strict stat", err)
	if disco, tokens, auths := script.snapshot(); disco+tokens+len(auths) != 0 {
		t.Fatalf("outbound for a withdrawn strict share: %d %d %d", disco, tokens, len(auths))
	}
}

// A withdrawal between two attempts stops the next attempt.
func TestWithdrawalBetweenRetriesStopsTheNextAttempt(t *testing.T) {
	ref := &provider.Reference{Path: "/share-abc"}
	t.Run("read retry", func(t *testing.T) {
		script := &scriptedSender{rejectAccessToken: true}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		gw := &switchingGateway{share: share, after: 1, next: withdrawnCopy(share)}
		stampGateway(gw)
		d := newTestReceivedDriver(t)
		ctx, _ := loggedContext(t)
		_, _, before := script.snapshot()
		_, err := d.GetMD(ctx, ref, nil)
		requireLocalDenial(t, "stat retry", err)
		_, _, after := script.snapshot()
		if len(after)-len(before) == 0 {
			t.Fatal("the first attempt never reached the sender")
		}
		// The retry looked the share up again and stopped there.
		if gw.lookups() != 2 {
			t.Fatalf("share lookups %d, want the first attempt plus the stopped retry", gw.lookups())
		}
	})
	t.Run("upload retry", func(t *testing.T) {
		script := &scriptedSender{rejectAccessToken: true}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		gw := &switchingGateway{share: share, after: 1, next: withdrawnCopy(share)}
		putsAt := -1
		countPuts := func() int {
			_, _, _, methods := script.snapshotWire()
			n := 0
			for _, m := range methods {
				if m == "PUT" {
					n++
				}
			}
			return n
		}
		gw.onSwitch = func() { putsAt = countPuts() }
		stampGateway(gw)
		d := newTestReceivedDriver(t)
		ctx, _ := loggedContext(t)
		err := d.Upload(ctx, ref, io.NopCloser(strings.NewReader("data")), nil)
		requireLocalDenial(t, "upload retry", err)
		if putsAt < 1 {
			t.Fatalf("first attempt did not upload before the withdrawal (%d)", putsAt)
		}
		if got := countPuts(); got != putsAt {
			t.Fatalf("PUTs after withdrawal: %d, at withdrawal %d", got, putsAt)
		}
	})
}

// An active hidden share stays usable: hiding is not revocation.
func TestHiddenActiveShareStaysUsable(t *testing.T) {
	script := &scriptedSender{}
	srv := startScriptedSender(t, script)
	share := scriptedShare(srv, false)
	share.Hidden = true
	stampGateway(&switchingGateway{share: share})
	d := newTestReceivedDriver(t)
	ctx, _ := loggedContext(t)
	if _, err := d.GetMD(ctx, &provider.Reference{Path: "/share-abc"}, nil); err != nil {
		t.Fatalf("hidden share: %v", err)
	}
}
