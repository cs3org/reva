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
	"context"
	"strings"
	"unicode"

	"google.golang.org/grpc/metadata"
)

const (
	// ReasonHeader is the gRPC metadata header that carries why an operator
	// makes an admin call. Audit writes it to every audit line.
	ReasonHeader = "x-reva-admin-reason"
	// SudoUserHeader carries the SUDO_USER of a local socket caller, as the
	// CLI reports it. The client sets it, so it is not trusted.
	SudoUserHeader = "x-reva-sudo-user"

	maxHeaderLen = 512
)

// Header returns an incoming gRPC metadata header, cleaned by Clean. The value
// comes from the caller, so it is only for logs, never for a decision.
func Header(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	return Clean(strings.Join(md.Get(key), "; "))
}

// Clean makes free text from a client safe for one log line: it removes
// control and format characters (bidi overrides can disguise a line), and
// cuts the text to a bounded length.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Cc, unicode.Cf) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	s = strings.TrimSpace(s)
	if len(s) > maxHeaderLen {
		// The cut can split the last rune; ToValidUTF8 drops the part.
		s = strings.ToValidUTF8(s[:maxHeaderLen], "")
	}
	return s
}

// ForwardReason adds the reason of the incoming call to the outgoing metadata,
// so that the owning service audits it too. Call it before an admin handler
// dials the control channel.
func ForwardReason(ctx context.Context) context.Context {
	r := Header(ctx, ReasonHeader)
	if r == "" {
		return ctx
	}
	if md, ok := metadata.FromOutgoingContext(ctx); ok && len(md.Get(ReasonHeader)) > 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, ReasonHeader, r)
}
