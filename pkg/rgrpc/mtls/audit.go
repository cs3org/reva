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

package mtls

import (
	"context"
	"net"
	"sync"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

// NewUnary logs, once per peer host and method, a call without a verified client certificate.
func NewUnary() grpc.UnaryServerInterceptor {
	a := &audit{}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		a.check(ctx, info.FullMethod)
		return handler(ctx, req)
	}
}

// NewStream is the streaming counterpart of NewUnary.
func NewStream() grpc.StreamServerInterceptor {
	a := &audit{}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		a.check(ss.Context(), info.FullMethod)
		return handler(srv, ss)
	}
}

// audit remembers the callers without a certificate it has already logged.
type audit struct {
	seen sync.Map
}

func (a *audit) check(ctx context.Context, method string) {
	p, ok := unverified(ctx)
	if !ok {
		return
	}
	host := p.Addr.String()
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if _, logged := a.seen.LoadOrStore(host+" "+method, struct{}{}); logged {
		return
	}
	var agent string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get("user-agent"); len(v) > 0 {
			agent = v[0]
		}
	}
	appctx.GetLogger(ctx).Warn().Str("peer", host).Str("method", method).Str("user_agent", agent).
		Str("auth", p.AuthInfo.AuthType()).Msg("grpc call without a verified client certificate")
}

// unverified returns the caller on ctx if it has no verified client certificate.
func unverified(ctx context.Context) (*peer.Peer, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, false
	}
	switch info := p.AuthInfo.(type) {
	case plaintext:
		return p, true
	case credentials.TLSInfo:
		return p, len(info.State.VerifiedChains) == 0
	}
	return nil, false
}
