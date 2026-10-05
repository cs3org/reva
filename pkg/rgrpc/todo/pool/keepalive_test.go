package pool

import (
	"context"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Keepalive is opt-in and driven by the keepalive time alone: anything that is
// not a usable positive duration leaves the parameters zero, which NewConn reads
// as "no keepalive dial option at all". A usable time without a usable timeout is
// the one case that gets a default, so that detection cannot be half configured.
func TestGetClientKeepaliveParams(t *testing.T) {
	const (
		unset = "<unset>" // no test value can collide with this
		want0 = time.Duration(0)
	)

	tests := map[string]struct {
		time, timeout       string
		wantTime, wantTimeo time.Duration
	}{
		"unset sends no pings at all": {
			time: unset, timeout: unset,
			wantTime: want0, wantTimeo: want0,
		},
		"a timeout alone does not enable keepalive": {
			time: unset, timeout: "5s",
			wantTime: want0, wantTimeo: want0,
		},
		"a time without a unit suffix reads as not configured": {
			time: "30", timeout: "5s",
			wantTime: want0, wantTimeo: want0,
		},
		"an empty time reads as not configured": {
			time: "", timeout: "5s",
			wantTime: want0, wantTimeo: want0,
		},
		"a negative time reads as not configured": {
			time: "-1s", timeout: "5s",
			wantTime: want0, wantTimeo: want0,
		},
		"a zero time reads as not configured": {
			time: "0s", timeout: "5s",
			wantTime: want0, wantTimeo: want0,
		},
		"both values are used as configured": {
			time: "30s", timeout: "5s",
			wantTime: 30 * time.Second, wantTimeo: 5 * time.Second,
		},
		"a time without a timeout falls back to the working default": {
			time: "30s", timeout: unset,
			wantTime: 30 * time.Second, wantTimeo: _defaultKeepaliveTimeout,
		},
		"a time with an unparseable timeout falls back to the working default": {
			time: "30s", timeout: "not a duration",
			wantTime: 30 * time.Second, wantTimeo: _defaultKeepaliveTimeout,
		},
		"a time with a negative timeout falls back to the working default": {
			time: "30s", timeout: "-5s",
			wantTime: 30 * time.Second, wantTimeo: _defaultKeepaliveTimeout,
		},
	}

	setenv := func(t *testing.T, name, value string) {
		t.Helper()
		if value == unset {
			// t.Setenv first, so its cleanup restores the ambient value - a
			// variable set in the environment the test runs in would otherwise
			// leak into the "not configured" cases
			t.Setenv(name, "")
			require.NoError(t, os.Unsetenv(name))
			return
		}
		t.Setenv(name, value)
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setenv(t, _clientKeepaliveTimeEnv, tt.time)
			setenv(t, _clientKeepaliveTimeoutEnv, tt.timeout)

			kp := GetClientKeepaliveParams()

			assert.Equal(t, tt.wantTime, kp.Time)
			assert.Equal(t, tt.wantTimeo, kp.Timeout)
			// an idle connection nobody uses does not need to be probed, and
			// not probing it keeps us clear of a server's ping enforcement
			assert.False(t, kp.PermitWithoutStream)
		})
	}
}

// A peer that stops answering on an established connection - a black-holed node,
// a wedged process - must fail the rpcs on that connection instead of parking
// them forever. Nothing in the grpc stack notices this on its own: the
// connection is up, the TCP writes succeed, and the answer simply never comes.
func TestClientKeepaliveDetectsABlackHoledPeer(t *testing.T) {
	if testing.Short() {
		t.Skip("grpc clamps the keepalive interval to 10s, so this test needs ~12s")
	}

	// 10s is the floor grpc enforces (internal.KeepaliveMinPingTime)
	t.Setenv(_clientKeepaliveTimeEnv, "10s")
	t.Setenv(_clientKeepaliveTimeoutEnv, "1s")

	backend := startHealthServer(t)
	proxy := startBlackHoleProxy(t, backend)

	conn, err := NewConn(proxy.addr())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := healthpb.NewHealthClient(conn)

	// establish the connection while the path is still healthy
	warmupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.Check(warmupCtx, &healthpb.HealthCheckRequest{})
	require.NoError(t, err, "the health server should be reachable through the proxy")

	proxy.blackHole()

	// deliberately no deadline on the call: the only thing that can make it
	// return is the keepalive giving up on the connection
	start := time.Now()
	_, err = client.Check(context.Background(), &healthpb.HealthCheckRequest{})
	elapsed := time.Since(start)

	require.Error(t, err, "the call must not succeed, the peer never answered")
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Less(t, elapsed, 25*time.Second, "took much longer than the keepalive time plus timeout")
}

// startHealthServer runs a stock grpc health server and returns its address.
func startHealthServer(t *testing.T) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	healthpb.RegisterHealthServer(s, health.NewServer())
	go func() {
		_ = s.Serve(lis)
	}()
	t.Cleanup(s.Stop)

	return lis.Addr().String()
}

// blackHoleProxy forwards tcp between a client and a backend until blackHole
// is called, after which it keeps both sockets open and reads from them but
// forwards nothing in either direction. A closed socket would be reported to
// the client right away; this is the failure mode that is not.
type blackHoleProxy struct {
	lis     net.Listener
	backend string
	blocked atomic.Bool

	mu    sync.Mutex
	conns []net.Conn
}

func startBlackHoleProxy(t *testing.T, backend string) *blackHoleProxy {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	p := &blackHoleProxy{lis: lis, backend: backend}
	go p.serve()

	t.Cleanup(func() {
		_ = lis.Close()
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, c := range p.conns {
			_ = c.Close()
		}
	})

	return p
}

func (p *blackHoleProxy) addr() string {
	return p.lis.Addr().String()
}

func (p *blackHoleProxy) blackHole() {
	p.blocked.Store(true)
}

func (p *blackHoleProxy) serve() {
	for {
		client, err := p.lis.Accept()
		if err != nil {
			return
		}

		backend, err := net.Dial("tcp", p.backend)
		if err != nil {
			_ = client.Close()
			return
		}

		p.mu.Lock()
		p.conns = append(p.conns, client, backend)
		p.mu.Unlock()

		go p.pipe(backend, client)
		go p.pipe(client, backend)
	}
}

func (p *blackHoleProxy) pipe(dst, src net.Conn) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 && !p.blocked.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}
