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

// Package mtls builds the credentials for mutual TLS between reva's gRPC services.
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// The modes a gRPC server runs in.
const (
	// Off serves plaintext only.
	Off = "off"
	// Permissive serves TLS and plaintext on one port, client certificate optional.
	Permissive = "permissive"
	// Strict serves TLS only, to callers with a verified client certificate.
	Strict = "strict"
)

// tlsHandshake is the first byte of every TLS connection; plaintext HTTP/2 opens with "PRI".
const tlsHandshake = 0x16

// ServerCredentials returns the transport credentials of a gRPC server in mode.
func ServerCredentials(mode string, c config.MTLS) (credentials.TransportCredentials, error) {
	switch mode {
	case "", Off:
		return insecure.NewCredentials(), nil
	case Permissive:
		conf, err := serverConfig(mode, c, tls.VerifyClientCertIfGiven)
		if err != nil {
			return nil, err
		}
		return &permissive{tls: credentials.NewTLS(conf)}, nil
	case Strict:
		conf, err := serverConfig(mode, c, tls.RequireAndVerifyClientCert)
		if err != nil {
			return nil, err
		}
		return credentials.NewTLS(conf), nil
	}
	return nil, fmt.Errorf("mtls: unknown mode %q, want %q, %q or %q", mode, Off, Permissive, Strict)
}

// Unprotected reports whether a server in mode, listening on addr, takes plaintext calls from other hosts.
func Unprotected(mode string, addr net.Addr) bool {
	if mode != "" && mode != Off {
		return false
	}
	tcp, ok := addr.(*net.TCPAddr)
	return ok && !tcp.IP.IsLoopback()
}

// ClientCredentials returns the transport credentials reva dials its peers with.
func ClientCredentials(c config.MTLS) (credentials.TransportCredentials, error) {
	if !c.OutgoingMTLS {
		return insecure.NewCredentials(), nil
	}
	cert, pool, err := load(c)
	if err != nil {
		return nil, fmt.Errorf("mtls: dialing with tls: %w", err)
	}
	return credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   c.ServerName,
	}), nil
}

func serverConfig(mode string, c config.MTLS, auth tls.ClientAuthType) (*tls.Config, error) {
	cert, pool, err := load(c)
	if err != nil {
		return nil, fmt.Errorf("mtls: %s mode: %w", mode, err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   auth,
	}, nil
}

// load reads the certificate reva presents and the CA it trusts its peers by.
func load(c config.MTLS) (tls.Certificate, *x509.CertPool, error) {
	if c.CertFile == "" || c.KeyFile == "" || c.CAFile == "" {
		return tls.Certificate{}, nil, errors.New("certfile, keyfile and cafile must be set in [shared.mtls]")
	}
	cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	ca, err := os.ReadFile(c.CAFile)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return tls.Certificate{}, nil, fmt.Errorf("no certificate found in %s", c.CAFile)
	}
	return cert, pool, nil
}

// permissive tells a TLS caller from a plaintext one by the first byte it sends.
type permissive struct {
	tls credentials.TransportCredentials
}

func (p *permissive) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	first := make([]byte, 1)
	if _, err := io.ReadFull(conn, first); err != nil {
		return nil, nil, err
	}
	conn = &prefixedConn{Conn: conn, prefix: first}
	if first[0] == tlsHandshake {
		return p.tls.ServerHandshake(conn)
	}
	return conn, plaintext{CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.NoSecurity}}, nil
}

func (p *permissive) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("mtls: permissive credentials are for servers only")
}

func (p *permissive) Info() credentials.ProtocolInfo {
	return p.tls.Info()
}

func (p *permissive) Clone() credentials.TransportCredentials {
	return &permissive{tls: p.tls.Clone()}
}

func (p *permissive) OverrideServerName(string) error {
	return nil
}

// plaintext is the AuthInfo of a caller a permissive server let in without TLS.
type plaintext struct {
	credentials.CommonAuthInfo
}

func (plaintext) AuthType() string {
	return "insecure"
}

// prefixedConn replays the bytes read off a connection before handing out the rest.
type prefixedConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixedConn) Read(b []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(b, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(b)
}
