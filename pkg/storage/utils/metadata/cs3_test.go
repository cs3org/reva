// Copyright 2018-2022 CERN
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

package metadata

import (
	"context"
	"net"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ctxpkg "github.com/owncloud/reva/v2/pkg/ctx"
	"github.com/owncloud/reva/v2/pkg/rgrpc/todo/pool"
)

// blockingGateway accepts the Authenticate call and then never answers it. It
// emulates a gateway that is itself stuck on a downstream storage provider:
// the TCP connection is established, but no response ever comes back.
type blockingGateway struct {
	gateway.UnimplementedGatewayAPIServer
	entered chan struct{}
}

func (g *blockingGateway) Authenticate(ctx context.Context, _ *gateway.AuthenticateRequest) (*gateway.AuthenticateResponse, error) {
	select {
	case g.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

// respondingGateway answers Authenticate immediately, like a healthy gateway.
type respondingGateway struct {
	gateway.UnimplementedGatewayAPIServer
}

func (g *respondingGateway) Authenticate(_ context.Context, _ *gateway.AuthenticateRequest) (*gateway.AuthenticateResponse, error) {
	return &gateway.AuthenticateResponse{
		Status: &rpc.Status{Code: rpc.Code_CODE_OK},
		Token:  "a-machine-auth-token",
	}, nil
}

// startGateway serves srv on a loopback port and returns its address.
func startGateway(t *testing.T, srv gateway.GatewayAPIServer) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	gateway.RegisterGatewayAPIServer(s, srv)
	go func() {
		_ = s.Serve(lis)
	}()

	addr := lis.Addr().String()
	t.Cleanup(func() {
		s.Stop()
		// the pool caches selectors globally, keyed by kind+address
		pool.RemoveSelector("GatewaySelector" + addr)
	})

	return addr
}

// systemUserCS3 builds a CS3 metadata storage that authenticates as a system
// user against gwAddr, the way NewCS3Storage does.
func systemUserCS3(gwAddr string, authTimeout time.Duration) *CS3 {
	cs3 := NewCS3(gwAddr, gwAddr)
	cs3.useSystemUser = true
	cs3.machineAuthAPIKey = "change-me-please"
	cs3.serviceUser = &user.User{
		Id: &user.UserId{
			OpaqueId: "some-system-user-id",
			Idp:      "internal",
		},
	}
	cs3.authRPCTimeout = authTimeout

	return cs3
}

// A gateway that never answers Authenticate must not be able to park a
// metadata operation forever. Without a deadline on that RPC the goroutine
// waits for the process lifetime, which is how a single stalled storage
// provider silently wedges the sharing service.
func TestAuthenticateRPCIsBounded(t *testing.T) {
	gw := &blockingGateway{entered: make(chan struct{}, 1)}
	cs3 := systemUserCS3(startGateway(t, gw), 200*time.Millisecond)

	errCh := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := cs3.Stat(context.Background(), "/some/path")
		errCh <- err
	}()

	select {
	case <-gw.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the Authenticate call never reached the gateway")
	}

	select {
	case err := <-errCh:
		elapsed := time.Since(start)
		require.Error(t, err, "Stat must fail when the gateway does not answer Authenticate")
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
		assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond, "returned before the timeout elapsed")
		assert.Less(t, elapsed, 3*time.Second, "took much longer than the configured timeout")
	case <-time.After(5 * time.Second):
		t.Fatal("cs3.Stat never returned: the Authenticate RPC is unbounded")
	}
}

// The context returned by getAuthContext is what the caller runs its own RPC
// on, so the auth timeout must not be attached to it - a cancelled or
// deadline-carrying context here would break every caller in cs3.go.
func TestGetAuthContextReturnsAUsableContext(t *testing.T) {
	cs3 := systemUserCS3(startGateway(t, &respondingGateway{}), 200*time.Millisecond)

	authCtx, err := cs3.getAuthContext(context.Background())
	require.NoError(t, err)
	require.NotNil(t, authCtx)

	assert.NoError(t, authCtx.Err(), "the returned context must still be alive")
	_, hasDeadline := authCtx.Deadline()
	assert.False(t, hasDeadline, "the auth RPC deadline must not leak into the returned context")

	md, ok := metadata.FromOutgoingContext(authCtx)
	require.True(t, ok, "the returned context must carry outgoing metadata")
	assert.Equal(t, []string{"a-machine-auth-token"}, md.Get(ctxpkg.TokenHeader))

	// the token of the caller's context must not survive
	tainted := metadata.AppendToOutgoingContext(context.Background(), ctxpkg.TokenHeader, "the-callers-token")
	authCtx, err = cs3.getAuthContext(tainted)
	require.NoError(t, err)
	md, _ = metadata.FromOutgoingContext(authCtx)
	assert.Equal(t, []string{"a-machine-auth-token"}, md.Get(ctxpkg.TokenHeader))
}

// Without a system user there is no Authenticate call at all and the caller's
// context has to be passed through untouched.
func TestGetAuthContextWithoutSystemUser(t *testing.T) {
	cs3 := NewCS3("127.0.0.1:1", "127.0.0.1:1")

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "value")
	authCtx, err := cs3.getAuthContext(ctx)
	require.NoError(t, err)
	assert.Equal(t, ctx, authCtx)
}
