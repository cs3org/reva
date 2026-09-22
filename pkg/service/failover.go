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
	"slices"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

const (
	// maxFailoverNodes bounds how many distinct nodes one call may try. A node
	// is tried once and then left alone: a refused connection will not
	// un-refuse within a request, and a node that is merely slow only gets
	// slower if we ask it again.
	maxFailoverNodes = 3

	// penaltyCooldown is how long a node that failed a call is passed over.
	penaltyCooldown = 30 * time.Second
)

// peerConn is the part of *grpc.ClientConn the resolver needs: the call surface
// the generated CS3 clients use, plus the connectivity state that says whether a
// failed call ever reached the peer.
type peerConn interface {
	grpc.ClientConnInterface
	GetState() connectivity.State
}

// failoverConn is the connection the CS3 clients are built on. It resolves a
// node per call, and a call that fails because its node could not be reached is
// sent to another node of the same service. The caller sees one client and one
// call.
type failoverConn struct {
	clients *clients
	service string
}

func (f *failoverConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	var tried []string
	var err error
	for attempt := range maxFailoverNodes {
		conn, addr, next := f.next(ctx, attempt, tried)
		if next != nil {
			setIfNil(&err, next)
			return err
		}
		tried = append(tried, addr)

		state := conn.GetState()
		err = conn.Invoke(ctx, method, args, reply, opts...)
		if err == nil {
			f.clients.reached(f.service, addr)
			return nil
		}
		if !retryElsewhere(err, state, method) {
			return err
		}
		f.clients.unreachable(ctx, f.service, addr, method, err)
	}
	return err
}

// NewStream fails over while opening the stream; once open, a stream that breaks
// is the caller's to retry, as its messages cannot be replayed from here.
func (f *failoverConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	var tried []string
	var err error
	for attempt := range maxFailoverNodes {
		conn, addr, next := f.next(ctx, attempt, tried)
		if next != nil {
			setIfNil(&err, next)
			return nil, err
		}
		tried = append(tried, addr)

		state := conn.GetState()
		var stream grpc.ClientStream
		stream, err = conn.NewStream(ctx, desc, method, opts...)
		if err == nil {
			f.clients.reached(f.service, addr)
			return stream, nil
		}
		if !retryElsewhere(err, state, method) {
			return nil, err
		}
		f.clients.unreachable(ctx, f.service, addr, method, err)
	}
	return nil, err
}

// next picks the node for one attempt, and a connection to it.
//
// The first attempt goes through the retrying lookup, so a peer that is still
// registering is waited for as it was before. A failover attempt must not: a
// service whose only node just failed is an ordinary dead end, not an
// unresolvable peer, and booking it as one would -- for the gateway -- end the
// process.
func (f *failoverConn) next(ctx context.Context, attempt int, tried []string) (peerConn, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if attempt == 0 {
		return f.clients.resolve(ctx, f.service)
	}
	return f.clients.resolveOther(f.service, tried)
}

// setIfNil keeps the first error of a call. What a caller wants to hear is why
// the call failed, not why the node after the one that failed could not be
// found.
func setIfNil(err *error, cause error) {
	if *err == nil {
		*err = cause
	}
}

// retryElsewhere reports whether a failed call may be sent to another node.
//
// The failure has to be a transport failure. Any other code is the peer's
// considered answer, and another node would answer the same.
//
// Whether the call may then be replayed depends on what the connection was doing
// when it broke. A connection that was not yet ready never handed the request
// over, so anything may be retried. A connection that was ready may have broken
// after the peer took the request, so only calls that are harmless to run twice
// are retried.
func retryElsewhere(err error, state connectivity.State, method string) bool {
	if status.Code(err) != codes.Unavailable {
		return false
	}
	return state != connectivity.Ready || idempotent(method)
}

// idempotentPrefixes name the CS3 verbs that only read, so a call that may or
// may not have reached a dying peer can be safely put to another one. Matching
// by prefix rather than by an explicit list of every method of twenty services
// keeps this from drifting as CS3 grows; the prefixes were checked against the
// whole current API surface, where they select reads only. A future method that
// reads *and* writes under one of these verbs -- a GetOrCreate, say -- would
// need excluding here.
var idempotentPrefixes = []string{"Check", "Find", "Get", "Has", "Is", "List", "Stat"}

// idempotentMethods are the reads whose names do not start with one of the
// prefixes above.
var idempotentMethods = []string{"WhoAmI"}

// idempotent reports whether a "/package.Service/Method" call is safe to run
// twice.
func idempotent(fullMethod string) bool {
	name := fullMethod[strings.LastIndex(fullMethod, "/")+1:]
	if slices.Contains(idempotentMethods, name) {
		return true
	}
	for _, prefix := range idempotentPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// The penalty box holds nodes this process failed to reach, so that the next
// call starts somewhere else instead of rediscovering the same dead peer.
//
// It is deliberately local. A node re-registers itself on every heartbeat with
// state=ready, and the liveness loop returns any node it still hears from to
// ready, so a "degraded" mark written to the shared registry cannot outlive one
// heartbeat interval. And the failure worth reacting to -- a peer that is
// healthy to the registry but unreachable from here -- is one only we can see.

func penaltyKey(service, address string) string { return service + "\x00" + address }

// unreachable notes that a node could not be reached and passes it over.
//
// Only the first failure of a cooldown is reported. A node that is the last one
// standing keeps being picked and keeps failing -- passing it over must never
// make its service unreachable -- so without this a dead peer under load would
// write a log line and a registry update per call.
func (c *clients) unreachable(ctx context.Context, service, address, method string, cause error) {
	if !c.penalize(service, address) {
		return
	}
	appctx.GetLogger(ctx).Warn().Err(cause).Str("service", service).Str("node", address).
		Str("method", method).Dur("cooldown", penaltyCooldown).
		Msg("peer unreachable, failing over to another node")
	// Off the request path, and best-effort: the registry may be the very thing
	// that is unreachable, and a hint for the other processes is not worth
	// making this call wait on a write.
	go c.markDegraded(service, address)
}

// penalize passes a node over for the cooldown, and reports whether it was not
// already being passed over.
func (c *clients) penalize(service, address string) bool {
	now := time.Now()
	c.penMu.Lock()
	defer c.penMu.Unlock()
	// Addresses come and go as peers are redeployed; drop the entries that have
	// served their time rather than keeping them for an address nobody holds.
	for key, until := range c.penalties {
		if now.After(until) {
			delete(c.penalties, key)
		}
	}
	key := penaltyKey(service, address)
	_, held := c.penalties[key]
	c.penalties[key] = now.Add(penaltyCooldown)
	return !held
}

// reached clears a node's penalty: it just answered.
func (c *clients) reached(service, address string) {
	c.penMu.Lock()
	defer c.penMu.Unlock()
	delete(c.penalties, penaltyKey(service, address))
}

func (c *clients) penalized(service, address string) bool {
	c.penMu.Lock()
	defer c.penMu.Unlock()
	until, ok := c.penalties[penaltyKey(service, address)]
	return ok && time.Now().Before(until)
}

// unpenalized drops the nodes this process recently failed to reach, unless that
// would leave none: a penalty is a preference, and must never be the reason a
// reachable service is reported unreachable.
func (c *clients) unpenalized(service string, nodes []registry.Node) []registry.Node {
	out := make([]registry.Node, 0, len(nodes))
	for _, n := range nodes {
		if !c.penalized(service, n.Address()) {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nodes
	}
	return out
}

// without drops the nodes a call has already tried.
func without(nodes []registry.Node, addresses []string) []registry.Node {
	if len(addresses) == 0 {
		return nodes
	}
	out := make([]registry.Node, 0, len(nodes))
	for _, n := range nodes {
		if !slices.Contains(addresses, n.Address()) {
			out = append(out, n)
		}
	}
	return out
}
