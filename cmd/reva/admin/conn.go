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

	"github.com/cs3org/reva/v3/cmd/reva/internal/cli"
	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// resolveAdminHost returns the admin endpoint from the flag, the environment
// or the config file.
func resolveAdminHost() (string, error) {
	if rt.AdminHost != "" {
		return rt.AdminHost, nil
	}
	return "", errors.New("admin host not set: pass --admin-host, set REVA_ADMIN_HOST or run `reva config set admin-host <host:port>`")
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
	if rt.Insecure || isSocketHost(host) {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsconf := &tls.Config{InsecureSkipVerify: rt.SkipVerify}
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
func adminAuthContext(ctx context.Context) (context.Context, error) {
	t, err := rt.AdminToken()
	if err != nil {
		return nil, err
	}
	return cli.WithToken(ctx, t), nil
}

// adminDial builds a client and its auth context. An admin host from the flag
// or the environment always wins. Else it prefers a working local-root socket,
// then falls back to the stored network host and the admin token.
func adminDial(cmd *cobra.Command) (adminpb.AdminAPIClient, context.Context, error) {
	ctx := cmd.Context()
	if cmd.Flags().Changed("admin-host") || os.Getenv("REVA_ADMIN_HOST") != "" {
		return adminDialHost(ctx, rt.AdminHost)
	}
	for _, p := range defaultSocketPaths() {
		if !isSocketFile(p) {
			continue
		}
		client, err := adminClientAt("unix://" + p)
		if err != nil {
			continue
		}
		if adminSocketWorks(ctx, client) {
			return client, ctx, nil
		}
	}
	host, err := resolveAdminHost()
	if err != nil {
		return nil, nil, err
	}
	return adminDialHost(ctx, host)
}

// adminDialHost dials one resolved host: a socket needs no token, a network
// host uses the stored one.
func adminDialHost(ctx context.Context, host string) (adminpb.AdminAPIClient, context.Context, error) {
	client, err := adminClientAt(host)
	if err != nil {
		return nil, nil, err
	}
	if isSocketHost(host) {
		return client, ctx, nil
	}
	ctx, err = adminAuthContext(ctx)
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
func adminSocketWorks(ctx context.Context, client adminpb.AdminAPIClient) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
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
		return fmt.Errorf("%w (admin token may be expired or missing; re-run `reva admin elevate`)", err)
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
