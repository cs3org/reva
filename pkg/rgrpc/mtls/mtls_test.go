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
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const serverName = "reva.test"

// What a server does with a caller.
const (
	refused  = "refused"
	reported = "let in, reported"
	silent   = "let in"
)

func TestServerModes(t *testing.T) {
	ca := newCA(t)
	certs := ca.leaf(t)
	rogue := newCA(t).leaf(t)
	rogue.CAFile = certs.CAFile

	callers := []struct {
		name  string
		creds func(t *testing.T) credentials.TransportCredentials
	}{
		{"plaintext", func(t *testing.T) credentials.TransportCredentials {
			return insecure.NewCredentials()
		}},
		{"tls without certificate", func(t *testing.T) credentials.TransportCredentials {
			return credentials.NewTLS(&tls.Config{RootCAs: ca.pool, ServerName: serverName})
		}},
		{"certificate of another ca", func(t *testing.T) credentials.TransportCredentials {
			creds, err := ClientCredentials(rogue)
			require.NoError(t, err)
			return creds
		}},
		{"certificate", func(t *testing.T) credentials.TransportCredentials {
			creds, err := ClientCredentials(certs)
			require.NoError(t, err)
			return creds
		}},
	}
	want := map[string][]string{
		Off:        {silent, refused, refused, refused},
		Permissive: {reported, reported, refused, silent},
		Strict:     {refused, refused, refused, silent},
	}

	for mode, outcomes := range want {
		for i, caller := range callers {
			t.Run(mode+"/"+caller.name, func(t *testing.T) {
				addr, unverified := serve(t, mode, certs)
				err := check(t, addr, caller.creds(t))
				got := refused
				if err == nil {
					got = silent
					if <-unverified {
						got = reported
					}
				}
				assert.Equal(t, outcomes[i], got, "call error: %v", err)
			})
		}
	}
}

func TestAuditLogsOncePerPeerAndMethod(t *testing.T) {
	out := &syncBuffer{}
	log := zerolog.New(out)
	creds, err := ServerCredentials(Permissive, newCA(t).leaf(t))
	require.NoError(t, err)
	addr := listen(t, grpc.NewServer(grpc.Creds(creds), grpc.ChainUnaryInterceptor(
		func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(appctx.WithLogger(ctx, &log), req)
		},
		NewUnary(),
	)))

	for range 2 {
		require.NoError(t, check(t, addr, insecure.NewCredentials()))
	}

	logged := out.String()
	assert.Equal(t, 1, strings.Count(logged, "grpc call without a verified client certificate"), logged)
	assert.Contains(t, logged, `"peer":"127.0.0.1"`)
	assert.Contains(t, logged, `"method":"/grpc.health.v1.Health/Check"`)
	assert.Contains(t, logged, `"auth":"insecure"`)
}

func TestCredentialsNeedCertificates(t *testing.T) {
	for _, mode := range []string{Permissive, Strict} {
		_, err := ServerCredentials(mode, config.MTLS{})
		assert.Error(t, err, mode)
	}
	_, err := ClientCredentials(config.MTLS{OutgoingMTLS: true})
	assert.Error(t, err)

	_, err = ServerCredentials("on", newCA(t).leaf(t))
	assert.Error(t, err)
}

func TestClientDialsPlaintextByDefault(t *testing.T) {
	creds, err := ClientCredentials(config.MTLS{})
	require.NoError(t, err)
	assert.Equal(t, "insecure", creds.Info().SecurityProtocol)
}

func TestUnprotected(t *testing.T) {
	anyHost := &net.TCPAddr{IP: net.IPv6unspecified, Port: 9142}
	loopback := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 9142}
	socket := &net.UnixAddr{Name: "/run/revad.sock", Net: "unix"}

	assert.True(t, Unprotected(Off, anyHost))
	assert.True(t, Unprotected("", anyHost))
	assert.False(t, Unprotected(Off, loopback))
	assert.False(t, Unprotected(Off, socket))
	assert.False(t, Unprotected(Permissive, anyHost))
	assert.False(t, Unprotected(Strict, anyHost))
}

// serve starts a health server in mode, reporting for each call it takes whether
// the caller came in unverified.
func serve(t *testing.T, mode string, certs config.MTLS) (string, <-chan bool) {
	creds, err := ServerCredentials(mode, certs)
	require.NoError(t, err)
	calls := make(chan bool, 1)
	return listen(t, grpc.NewServer(grpc.Creds(creds), grpc.UnaryInterceptor(
		func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			_, ok := unverified(ctx)
			calls <- ok
			return handler(ctx, req)
		},
	))), calls
}

func listen(t *testing.T, s *grpc.Server) string {
	healthpb.RegisterHealthServer(s, health.NewServer())
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = s.Serve(ln) }()
	t.Cleanup(s.Stop)
	return ln.Addr().String()
}

func check(t *testing.T, addr string, creds credentials.TransportCredentials) error {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
	return err
}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
	file string
}

func newCA(t *testing.T) *testCA {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "reva test ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	file := filepath.Join(t.TempDir(), "ca.pem")
	writePEM(t, file, "CERTIFICATE", der)
	return &testCA{cert: cert, key: key, pool: pool, file: file}
}

// leaf issues a certificate for both ends of a connection, as reva shares one.
func (ca *testCA) leaf(t *testing.T) config.MTLS {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: serverName},
		DNSNames:     []string{serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	dir := t.TempDir()
	c := config.MTLS{
		CertFile:     filepath.Join(dir, "cert.pem"),
		KeyFile:      filepath.Join(dir, "key.pem"),
		CAFile:       ca.file,
		ServerName:   serverName,
		OutgoingMTLS: true,
	}
	writePEM(t, c.CertFile, "CERTIFICATE", der)
	writePEM(t, c.KeyFile, "PRIVATE KEY", keyDER)
	return c
}

func writePEM(t *testing.T, path, kind string, der []byte) {
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600))
}

// syncBuffer is written by the server's goroutines and read by the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
