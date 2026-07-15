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

package admin

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// resolveAdminHost returns the admin endpoint, persisting a -admin-host flag via
// the caller's callback when given so it need not be repeated.
func resolveAdminHost(flagVal string) (string, error) {
	if flagVal != "" {
		cliOpts.PersistAdminHost(flagVal)
		return flagVal, nil
	}
	if h := cliOpts.AdminHost(); h != "" {
		return h, nil
	}
	return "", errors.New("admin host not set: pass -admin-host <host:port> once (it is persisted for next time)")
}

// isSocketHost reports whether the admin host is a local Unix socket
// ("unix:///..."), dialed without TLS and without a token.
func isSocketHost(host string) bool { return strings.HasPrefix(host, "unix:") }

// adminMaxRecvMsgSize lifts the 4 MiB default so a fleet-wide unary fan-out
// (many instances' results in one response) has headroom. Large per-instance
// results (e.g. stack dumps) should stream instead — see `admin stack`.
const adminMaxRecvMsgSize = 64 << 20

func adminConn(host string) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(adminMaxRecvMsgSize))}
	if cliOpts.Insecure || isSocketHost(host) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsconf := &tls.Config{InsecureSkipVerify: cliOpts.SkipVerify}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsconf)))
	}
	return grpc.NewClient(host, opts...)
}

func adminClientAt(host string) (adminpb.AdminAPIClient, error) {
	conn, err := adminConn(host)
	if err != nil {
		return nil, err
	}
	return adminpb.NewAdminAPIClient(conn), nil
}

// adminAuthContext attaches the stored short-TTL admin token. Every subcommand
// except `elevate` uses it.
func adminAuthContext() (context.Context, error) {
	t, err := readAdminToken()
	if err != nil || t == "" {
		return nil, errors.New("no admin token found: run `admin elevate` first")
	}
	ctx := appctx.ContextSetToken(context.Background(), t)
	ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, t)
	return ctx, nil
}

// adminDial builds a client and its auth context. With no -admin-host it
// prefers a working local-root socket, then falls back to the stored network
// host + admin token. An explicit -admin-host always wins.
func adminDial(adminHostFlag string) (adminpb.AdminAPIClient, context.Context, error) {
	if adminHostFlag != "" {
		host, err := resolveAdminHost(adminHostFlag)
		if err != nil {
			return nil, nil, err
		}
		return adminDialHost(host)
	}

	// No flag: try the local-root socket(s) first.
	for _, p := range defaultSocketPaths() {
		if !isSocketFile(p) {
			continue
		}
		client, err := adminClientAt("unix://" + p)
		if err != nil {
			continue
		}
		if adminSocketWorks(client) {
			return client, context.Background(), nil
		}
	}

	// Fall back to the stored network admin host + token.
	host, err := resolveAdminHost("")
	if err != nil {
		return nil, nil, err
	}
	return adminDialHost(host)
}

// adminDialHost dials one resolved host: a socket needs no token, a network
// host uses the stored one.
func adminDialHost(host string) (adminpb.AdminAPIClient, context.Context, error) {
	client, err := adminClientAt(host)
	if err != nil {
		return nil, nil, err
	}
	if isSocketHost(host) {
		return client, context.Background(), nil
	}
	ctx, err := adminAuthContext()
	if err != nil {
		return nil, nil, err
	}
	return client, ctx, nil
}

// defaultSocketPaths mirrors the server's well-known socket locations (the CLI
// cannot import the service package).
func defaultSocketPaths() []string {
	paths := []string{"/run/reva/admin.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "reva", "admin.sock"))
	}
	return paths
}

func isSocketFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}

// adminSocketWorks probes the socket with a cheap call; a denied or stale
// socket returns false so the caller falls back.
func adminSocketWorks(client adminpb.AdminAPIClient) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := client.GetServerInfo(ctx, &adminpb.GetServerInfoRequest{})
	return err == nil
}

// adminErr annotates authn/authz failures with the sudo-timeout hint.
func adminErr(err error) error {
	if err == nil {
		return nil
	}
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		return fmt.Errorf("%w (admin token may be expired or missing; re-run `admin elevate`)", err)
	default:
		return err
	}
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}
