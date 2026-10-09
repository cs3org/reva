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
	// A node is tried once: a refused connection will not un-refuse within a
	// request, and a node that is merely slow only gets slower if we ask again.
	maxFailoverNodes = 3

	penaltyCooldown = 30 * time.Second
)

// peerConn is the part of *grpc.ClientConn the resolver needs.
type peerConn interface {
	grpc.ClientConnInterface
	GetState() connectivity.State
}

// failoverConn is the connection the CS3 clients are built on. It resolves a
// node per call, and a call that fails because its node could not be reached is
// sent to another node of the same service.
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
		retry := retryElsewhere(err, state, method)
		blame := retry && lost(conn)
		traceFailure(ctx, f.service, addr, method, attempt, state, err, retry, blame)
		if !retry {
			return err
		}
		if blame {
			f.clients.unreachable(ctx, f.service, addr, method, err)
		}
	}
	return err
}

// NewStream fails over while opening the stream. Once open, a stream that breaks
// is the caller's to retry: its messages cannot be replayed from here.
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
		retry := retryElsewhere(err, state, method)
		blame := retry && lost(conn)
		traceFailure(ctx, f.service, addr, method, attempt, state, err, retry, blame)
		if !retry {
			return nil, err
		}
		if blame {
			f.clients.unreachable(ctx, f.service, addr, method, err)
		}
	}
	return nil, err
}

// traceFailure records what a failed attempt led to: whether the call moves on
// to another node, and whether this one is passed over for it.
func traceFailure(ctx context.Context, service, address, method string, attempt int, state connectivity.State, err error, retry, blame bool) {
	appctx.GetLogger(ctx).Trace().Err(err).Str("peer", service).Str("node", address).
		Str("method", method).Int("attempt", attempt).Stringer("state_before", state).
		Bool("retry_elsewhere", retry).Bool("blamed", blame).Msg("failed attempt")
}

// next picks the node for one attempt. Only the first goes through the retrying
// lookup: a service whose only node just failed is an ordinary dead end, not an
// unresolvable peer, and booking it as one would -- for the gateway -- end the
// process.
func (f *failoverConn) next(ctx context.Context, attempt int, tried []string) (peerConn, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if attempt == 0 {
		return f.clients.resolve(ctx, f.service)
	}
	return f.clients.resolveOther(ctx, f.service, tried)
}

// setIfNil keeps the first error of a call: what a caller wants to hear is why
// the call failed, not why no further node was found.
func setIfNil(err *error, cause error) {
	if *err == nil {
		*err = cause
	}
}

// retryElsewhere reports whether a failed call may be sent to another node. Any
// code other than Unavailable is the peer's answer, and another node would answer
// the same. A connection that was not yet ready never handed the request over, so
// anything may be retried; one that was ready may have broken after the peer took
// the request, so only reads are.
func retryElsewhere(err error, state connectivity.State, method string) bool {
	if status.Code(err) != codes.Unavailable {
		return false
	}
	return state != connectivity.Ready || idempotent(method)
}

// lost reports whether a failed call left its node unreachable. A connection
// still up means the node answered: the Unavailable came from further down, a
// gateway relaying a storage provider that is down, say. Another node may do
// better, but this one is not to blame, and passing it over would turn one
// broken backend into every caller in the process losing the service.
func lost(conn peerConn) bool {
	return conn.GetState() != connectivity.Ready
}

// Matching by prefix keeps this from drifting as CS3 grows. The prefixes were
// checked against the whole current API surface, where they select reads only; a
// future method that reads *and* writes under one of them -- a GetOrCreate, say
// -- would need excluding.
var (
	idempotentPrefixes = []string{"Check", "Find", "Get", "Has", "Is", "List", "Stat"}
	idempotentMethods  = []string{"WhoAmI"}
)

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

// The penalty box holds the nodes this process failed to reach, so that the next
// call starts somewhere else. It is local because a node re-registers itself as
// ready on every heartbeat: a mark in the shared registry cannot outlive one
// heartbeat interval, and a peer that is healthy to the registry but unreachable
// from here is one only we can see.

func penaltyKey(service, address string) string { return service + "\x00" + address }

// unreachable passes a node over and reports it, but only on the first failure of
// a cooldown: a node that is the last one standing keeps being picked and keeps
// failing, which would otherwise log and write to the registry on every call.
func (c *clients) unreachable(ctx context.Context, service, address, method string, cause error) {
	if !c.penalize(service, address) {
		return
	}
	appctx.GetLogger(ctx).Warn().Err(cause).Str("service", service).Str("node", address).
		Str("method", method).Dur("cooldown", penaltyCooldown).
		Msg("peer unreachable, failing over to another node")
	// Off the request path: the registry may be the very thing that is
	// unreachable, and a hint for the other processes is not worth waiting on.
	go c.markDegraded(service, address)
}

// penalize passes a node over for the cooldown, and reports whether it was not
// already being passed over.
func (c *clients) penalize(service, address string) bool {
	now := time.Now()
	c.penMu.Lock()
	defer c.penMu.Unlock()
	// Addresses come and go as peers are redeployed, so drop what has expired
	// rather than keeping it for an address nobody holds.
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

// penalizedOf lists the addresses of nodes that are being passed over.
func (c *clients) penalizedOf(service string, nodes []registry.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if c.penalized(service, n.Address()) {
			out = append(out, n.Address())
		}
	}
	return out
}

// unpenalized drops the penalized nodes, unless that would leave none: a penalty
// is a preference, and must never be the reason a reachable service is reported
// unreachable.
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
