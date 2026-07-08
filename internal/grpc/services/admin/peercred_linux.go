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

//go:build linux

package admin

import (
	"context"
	"errors"
	"net"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc/credentials"
)

// peerCredCreds is a gRPC transport credential for Unix sockets: no encryption,
// but it captures the peer's OS credentials via SO_PEERCRED. The kernel fills
// them in, so they cannot be forged.
type peerCredCreds struct{}

func (peerCredCreds) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return conn, nil, errors.New("admin: control socket accepted a non-unix connection")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return conn, nil, err
	}
	var cred *unix.Ucred
	var serr error
	if cerr := raw.Control(func(fd uintptr) {
		cred, serr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); cerr != nil {
		return conn, nil, cerr
	}
	if serr != nil {
		return conn, nil, serr
	}
	info := peerCredAuthInfo{
		CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.PrivacyAndIntegrity},
		cred:           peerCred{uid: cred.Uid, gid: cred.Gid, pid: cred.Pid},
	}
	return conn, info, nil
}

func (peerCredCreds) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return nil, nil, errors.New("admin: peercred client handshake is not supported")
}

func (peerCredCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "peercred"}
}

func (peerCredCreds) Clone() credentials.TransportCredentials { return peerCredCreds{} }
func (peerCredCreds) OverrideServerName(string) error         { return nil }

// peerCredentials returns the transport credential that captures SO_PEERCRED.
func peerCredentials() credentials.TransportCredentials { return peerCredCreds{} }

// localRootSupported reports whether SO_PEERCRED local root is available here.
func localRootSupported() bool { return true }
