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

package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/registry/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

const (
	nodeA = "10.0.0.1:1"
	nodeB = "10.0.0.2:1"
	nodeC = "10.0.0.3:1"
	nodeD = "10.0.0.4:1"

	readMethod  = "/cs3.gateway.v1beta1.GatewayAPI/Stat"
	writeMethod = "/cs3.gateway.v1beta1.GatewayAPI/CreateShare"
)

// fakePeer stands in for a node's connection: it records the calls it is given
// and answers with whatever the test asked for.
type fakePeer struct {
	address string
	state   connectivity.State
	answer  func(address string) error
	calls   *callLog
}

func (p *fakePeer) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	p.calls.add(p.address)
	return p.answer(p.address)
}

func (p *fakePeer) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	p.calls.add(p.address)
	return nil, p.answer(p.address)
}

func (p *fakePeer) GetState() connectivity.State { return p.state }

type callLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *callLog) add(address string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, address)
}

func (l *callLog) addresses() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.seen)
}

// byAddress picks the lowest address, so that a test knows which node a call
// lands on: the registry hands its nodes back in map order.
type byAddress struct{}

func (byAddress) Pick(nodes []registry.Node) (registry.Node, bool) {
	candidates := selectable(nodes)
	if len(candidates) == 0 {
		return nil, false
	}
	return slices.MinFunc(candidates, func(a, b registry.Node) int {
		return strings.Compare(a.Address(), b.Address())
	}), true
}

// peers builds a resolver whose gateway has the given nodes, and whose calls are
// answered by answer instead of leaving the process.
func peers(state connectivity.State, answer func(address string) error, addresses ...string) (*clients, *callLog) {
	reg := memory.New(nil)
	nodes := make([]registry.Node, 0, len(addresses))
	for i, addr := range addresses {
		nodes = append(nodes, registry.NewNode(string(rune('a'+i)), addr, meta(registry.StateReady)))
	}
	_ = reg.Add(registry.NewService(NameGateway, nodes))

	calls := &callLog{}
	c := NewClients(reg).(*clients)
	c.selector = byAddress{}
	c.open = func(address string) (peerConn, error) {
		return &fakePeer{address: address, state: state, answer: answer, calls: calls}, nil
	}
	return c, calls
}

func unavailableAt(addresses ...string) func(string) error {
	return func(address string) error {
		if slices.Contains(addresses, address) {
			return status.Error(codes.Unavailable, "connection refused")
		}
		return nil
	}
}

func invoke(t *testing.T, c *clients, method string) error {
	t.Helper()
	conn, err := c.conn(context.Background(), NameGateway)
	if err != nil {
		return err
	}
	return conn.Invoke(context.Background(), method, nil, nil)
}

func TestFailoverMovesToTheNextNode(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(nodeA), nodeA, nodeB, nodeC)

	if err := invoke(t, c, writeMethod); err != nil {
		t.Fatalf("expected the call to succeed on another node: %v", err)
	}
	if got := calls.addresses(); !slices.Equal(got, []string{nodeA, nodeB}) {
		t.Fatalf("expected the call to move from %s to %s, got %v", nodeA, nodeB, got)
	}
}

func TestFailoverStopsAfterMaxNodes(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(nodeA, nodeB, nodeC, nodeD), nodeA, nodeB, nodeC, nodeD)

	err := invoke(t, c, writeMethod)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected the transport failure to surface, got %v", err)
	}
	if got := calls.addresses(); len(got) != maxFailoverNodes {
		t.Fatalf("expected %d nodes to be tried, got %v", maxFailoverNodes, got)
	}
}

func TestFailoverTriesEachNodeOnce(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(nodeA, nodeB, nodeC), nodeA, nodeB, nodeC)

	_ = invoke(t, c, writeMethod)
	got := calls.addresses()
	if len(got) != len(slices.Compact(slices.Sorted(slices.Values(got)))) {
		t.Fatalf("expected each node to be tried once, got %v", got)
	}
}

func TestOnlyTransportFailuresMoveOn(t *testing.T) {
	denied := status.Error(codes.PermissionDenied, "nope")
	c, calls := peers(connectivity.Ready, func(string) error { return denied }, nodeA, nodeB, nodeC)

	err := invoke(t, c, readMethod)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected the peer's answer to surface, got %v", err)
	}
	if got := calls.addresses(); len(got) != 1 {
		t.Fatalf("an answered call must not be repeated elsewhere, got %v", got)
	}
}

// A connection that was ready may have broken after the peer took the request,
// so a write is not replayed while a read is.
func TestAReadyConnectionOnlyReplaysReads(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		want   []string
	}{
		{"read", readMethod, []string{nodeA, nodeB}},
		{"write", writeMethod, []string{nodeA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, calls := peers(connectivity.Ready, unavailableAt(nodeA), nodeA, nodeB, nodeC)
			_ = invoke(t, c, tc.method)
			if got := calls.addresses(); !slices.Equal(got, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestAnUnreachableNodeIsPassedOverNextTime(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(nodeA), nodeA, nodeB, nodeC)

	if err := invoke(t, c, writeMethod); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := invoke(t, c, writeMethod); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if got := calls.addresses(); !slices.Equal(got, []string{nodeA, nodeB, nodeB}) {
		t.Fatalf("expected the second call to skip %s, got %v", nodeA, got)
	}
}

// A node that is the last one standing keeps being picked and keeps failing, so
// only the first failure of a cooldown is worth reporting.
func TestOnlyTheFirstFailureOfACooldownIsReported(t *testing.T) {
	c, _ := peers(connectivity.Idle, unavailableAt(), nodeA)

	if !c.penalize(NameGateway, nodeA) {
		t.Fatal("the first failure must be reported")
	}
	for range 5 {
		if c.penalize(NameGateway, nodeA) {
			t.Fatal("a node already being passed over must not be reported again")
		}
	}
	c.reached(NameGateway, nodeA)
	if !c.penalize(NameGateway, nodeA) {
		t.Fatal("a node that recovered and failed again must be reported")
	}
}

func TestAReachedNodeLosesItsPenalty(t *testing.T) {
	c, _ := peers(connectivity.Idle, unavailableAt(), nodeA, nodeB)
	c.penalize(NameGateway, nodeA)

	if !c.penalized(NameGateway, nodeA) {
		t.Fatal("expected the node to be penalized")
	}
	c.reached(NameGateway, nodeA)
	if c.penalized(NameGateway, nodeA) {
		t.Fatal("a node that answered must not stay penalized")
	}
}

func TestAPenaltyExpires(t *testing.T) {
	c, _ := peers(connectivity.Idle, unavailableAt(), nodeA)
	c.penMu.Lock()
	c.penalties[penaltyKey(NameGateway, nodeA)] = time.Now().Add(-time.Second)
	c.penMu.Unlock()

	if c.penalized(NameGateway, nodeA) {
		t.Fatal("a penalty past its cooldown must not hold")
	}
}

// A penalty is a preference. It must never be the reason a reachable service is
// reported unreachable.
func TestEveryNodePenalizedStillResolves(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(), nodeA, nodeB)
	c.penalize(NameGateway, nodeA)
	c.penalize(NameGateway, nodeB)

	if err := invoke(t, c, writeMethod); err != nil {
		t.Fatalf("expected the call to go through anyway: %v", err)
	}
	if got := calls.addresses(); len(got) != 1 {
		t.Fatalf("expected one call, got %v", got)
	}
}

func TestPenaltiesAreScopedToOneService(t *testing.T) {
	c, _ := peers(connectivity.Idle, unavailableAt(), nodeA)
	c.penalize(NameGateway, nodeA)

	if c.penalized(NameStorageProvider, nodeA) {
		t.Fatal("a penalty must not follow an address across services")
	}
}

// Failing over is not the same as being unable to find a peer. A gateway with a
// single unreachable node must fail its calls, not end the process.
func TestFailoverNeverEndsTheProcess(t *testing.T) {
	restore := exit
	var reason string
	exit = func(r string) { reason = r }
	defer func() { exit = restore }()

	c, _ := peers(connectivity.Idle, unavailableAt(nodeA), nodeA)
	c.fails[NameGateway] = &failure{first: time.Now().Add(-2 * unresolvableFor), calls: unresolvableCalls}

	for range 10 {
		if err := invoke(t, c, writeMethod); status.Code(err) != codes.Unavailable {
			t.Fatalf("expected the call to fail, got %v", err)
		}
	}
	if reason != "" {
		t.Fatalf("a node that cannot be reached must not end the process: %v", reason)
	}
}

func TestACancelledCallStopsFailingOver(t *testing.T) {
	c, calls := peers(connectivity.Idle, unavailableAt(nodeA, nodeB, nodeC), nodeA, nodeB, nodeC)
	conn, err := c.conn(context.Background(), NameGateway)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := conn.Invoke(ctx, writeMethod, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation to surface, got %v", err)
	}
	if got := calls.addresses(); len(got) != 0 {
		t.Fatalf("a cancelled call must reach no node, got %v", got)
	}
}

func TestTheFirstFailureIsTheOneReported(t *testing.T) {
	c, _ := peers(connectivity.Idle, unavailableAt(nodeA), nodeA)

	// Only one node, so the failover finds nothing: the caller must still hear
	// why the call failed, not that no second node exists.
	err := invoke(t, c, writeMethod)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected the transport failure, got %v", err)
	}
}

func TestRetryElsewhere(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "refused")
	for _, tc := range []struct {
		name   string
		err    error
		state  connectivity.State
		method string
		want   bool
	}{
		{"unreached peer, write", unavailable, connectivity.TransientFailure, writeMethod, true},
		{"unreached peer, read", unavailable, connectivity.Idle, readMethod, true},
		{"ready peer, read", unavailable, connectivity.Ready, readMethod, true},
		{"ready peer, write", unavailable, connectivity.Ready, writeMethod, false},
		{"peer answered", status.Error(codes.NotFound, "gone"), connectivity.Ready, readMethod, false},
		{"deadline", status.Error(codes.DeadlineExceeded, "slow"), connectivity.Idle, readMethod, false},
		{"plain error", errors.New("boom"), connectivity.Idle, readMethod, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryElsewhere(tc.err, tc.state, tc.method); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestIdempotent(t *testing.T) {
	// Checked against the CS3 API surface: every method below exists.
	reads := []string{
		"Stat", "ListContainer", "ListContainerStream", "GetUser", "GetHome", "GetQuota",
		"GetPath", "GetAppProviders", "CheckPermission", "FindUsers", "HasMember",
		"IsProviderAllowed", "ListRecycle", "WhoAmI",
	}
	writes := []string{
		"CreateShare", "Move", "Delete", "InitiateFileUpload", "InitiateFileDownload",
		"Authenticate", "SetArbitraryMetadata", "UpdateShare", "RestoreRecycleItem",
		"PurgeRecycle", "TouchFile", "RefreshLock", "OpenInApp", "GenerateInviteToken",
		"RemoveShare", "CreateHome", "AddGrant", "SetLock", "Unlock", "CreateStorageSpace",
	}
	for _, name := range reads {
		if !idempotent("/cs3.gateway.v1beta1.GatewayAPI/" + name) {
			t.Errorf("%s only reads, it should be replayable", name)
		}
	}
	for _, name := range writes {
		if idempotent("/cs3.gateway.v1beta1.GatewayAPI/" + name) {
			t.Errorf("%s may write, it must not be replayed", name)
		}
	}
}

func TestWithout(t *testing.T) {
	nodes := []registry.Node{
		registry.NewNode("a", nodeA, meta(registry.StateReady)),
		registry.NewNode("b", nodeB, meta(registry.StateReady)),
	}
	if got := without(nodes, nil); len(got) != 2 {
		t.Fatalf("expected both nodes, got %d", len(got))
	}
	got := without(nodes, []string{nodeA})
	if len(got) != 1 || got[0].Address() != nodeB {
		t.Fatalf("expected only %s, got %v", nodeB, got)
	}
}
