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

// Package grpc is the backend of the CLI over CS3 gRPC to the gateway.
package grpc

import (
	"context"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"google.golang.org/grpc"
)

// GatewayFunc returns the gateway client, and ctx with the reva token of the
// user plane attached.
type GatewayFunc func(ctx context.Context) (gateway.GatewayAPIClient, context.Context, error)

// Backend implements backend.Backend, and the optional abilities
// PreferenceStore and TransferManager. The gateway has no ListGrants, so it
// is no GrantLister.
type Backend struct {
	gw GatewayFunc
}

// New returns the backend. gw runs at each call, so the gateway is dialed
// only when a command needs it.
func New(gw GatewayFunc) *Backend { return &Backend{gw: gw} }

var (
	_ backend.Backend         = (*Backend)(nil)
	_ backend.PreferenceStore = (*Backend)(nil)
	_ backend.TransferManager = (*Backend)(nil)
)

func (b *Backend) Transport() string          { return backend.GRPC }
func (b *Backend) Files() backend.Files       { return files{b} }
func (b *Backend) Shares() backend.Shares     { return shares{b} }
func (b *Backend) Links() backend.Links       { return links{b} }
func (b *Backend) Spaces() backend.Spaces     { return spaces{b} }
func (b *Backend) Trash() backend.Trash       { return trash{b} }
func (b *Backend) Versions() backend.Versions { return versions{b} }
func (b *Backend) Locks() backend.Locks       { return locks{b} }
func (b *Backend) OCM() backend.OCM           { return ocmArea{b} }
func (b *Backend) Tokens() backend.Tokens     { return tokens{b} }
func (b *Backend) Users() backend.Users       { return users{b} }
func (b *Backend) Locator() backend.Locator   { return locator{b} }

// ServerTime is not in CS3.
func (b *Backend) ServerTime(ctx context.Context) (time.Time, error) {
	return time.Time{}, backend.NotAvailable(backend.GRPC, "the server time")
}

type response interface{ GetStatus() *rpc.Status }

func check[R response](res R, err error) (R, error) {
	if err != nil {
		return res, err
	}
	return res, backend.Check(res.GetStatus())
}

// call runs one gateway RPC with the user token, and checks the CS3 status.
func call[Req any, R response](ctx context.Context, b *Backend, rpc func(gateway.GatewayAPIClient, context.Context, Req, ...grpc.CallOption) (R, error), req Req) (R, error) {
	c, ctx, err := b.gw(ctx)
	if err != nil {
		var zero R
		return zero, err
	}
	return check(rpc(c, ctx, req))
}

func notAvailable(what string) error { return backend.NotAvailable(backend.GRPC, what) }
