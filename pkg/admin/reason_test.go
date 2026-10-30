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
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/metadata"
)

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"INC0042 fix quota":            "INC0042 fix quota",
		"line\n{\"audit\":true}\r":     "line{\"audit\":true}",
		"\x1b[31mred\x1b[0m":           "[31mred[0m",
		"admin‮gnp.exe":                "admingnp.exe",
		"  padded  ":                   "padded",
		"bad\xffutf8":                  "badutf8",
		strings.Repeat("a", 600):       strings.Repeat("a", maxHeaderLen),
		"a" + strings.Repeat("é", 300): "a" + strings.Repeat("é", 255),
	} {
		got := Clean(in)
		if got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
		if !utf8.ValidString(got) || len(got) > maxHeaderLen {
			t.Errorf("Clean(%q) = %q is not valid UTF-8 of at most %d bytes", in, got, maxHeaderLen)
		}
	}
}

func auditLine(t *testing.T, ctx context.Context) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	Audit(appctx.WithLogger(ctx, &log), AuditEvent{Action: "test"})
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestAuditReason(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(ReasonHeader, "INC0042\nforged"))
	if got := auditLine(t, ctx)["reason"]; got != "INC0042forged" {
		t.Errorf("reason = %v, want the cleaned header", got)
	}
	if _, ok := auditLine(t, context.Background())["reason"]; ok {
		t.Error("an audit line without the header must have no reason")
	}
}

func TestForwardReason(t *testing.T) {
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(ReasonHeader, "INC0042"))
	ctx = ForwardReason(ForwardReason(ctx))
	md, _ := metadata.FromOutgoingContext(ctx)
	if got := md.Get(ReasonHeader); len(got) != 1 || got[0] != "INC0042" {
		t.Errorf("outgoing reason = %v, want one INC0042", got)
	}
	if _, ok := metadata.FromOutgoingContext(ForwardReason(context.Background())); ok {
		t.Error("no reason must add no outgoing metadata")
	}
}
