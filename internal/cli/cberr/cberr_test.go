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

package cberr

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"op and path", &Error{Kind: KindNotFound, Op: "stat", Path: "/eos/user/g/x", Msg: "no such file or directory"},
			"cannot stat /eos/user/g/x: no such file or directory"},
		{"op only", &Error{Kind: KindAuth, Op: "authenticate", Msg: "ticket expired"},
			"cannot authenticate: ticket expired"},
		{"message only", &Error{Kind: KindUsage, Msg: "two local paths"},
			"two local paths"},
		{"kind supplies default", &Error{Kind: KindPermission},
			"permission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestExitCodes pins the codes documented in the README. Scripts depend on
// them, so a change here is a breaking change.
func TestExitCodes(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("boom"), 1},
		{&Error{Kind: KindOther}, 1},
		{&Error{Kind: KindUsage}, 2},
		{&Error{Kind: KindAuth}, 3},
		{&Error{Kind: KindPermission}, 4},
		{&Error{Kind: KindNotFound}, 5},
		{&Error{Kind: KindConflict}, 6},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.err); got != tt.want {
			t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestExitCodeThroughWrapping(t *testing.T) {
	inner := &Error{Kind: KindPermission, Msg: "denied"}
	wrapped := fmt.Errorf("uploading chunk 3: %w", inner)
	if got := ExitCode(wrapped); got != ExitPermission {
		t.Errorf("ExitCode through fmt.Errorf = %d, want %d", got, ExitPermission)
	}
}

// TestAuthAndPermissionStayDistinct guards the distinction scripts rely on:
// 401 is worth retrying after kinit, 403 never is.
func TestAuthAndPermissionStayDistinct(t *testing.T) {
	unauth := FromStatus(http.StatusUnauthorized, "stat", "/eos/x", "")
	forbidden := FromStatus(http.StatusForbidden, "stat", "/eos/x", "")

	if ExitCode(unauth) != ExitAuth {
		t.Errorf("401 mapped to exit %d, want %d", ExitCode(unauth), ExitAuth)
	}
	if ExitCode(forbidden) != ExitPermission {
		t.Errorf("403 mapped to exit %d, want %d", ExitCode(forbidden), ExitPermission)
	}
	if ExitCode(unauth) == ExitCode(forbidden) {
		t.Fatal("401 and 403 must not collapse to the same exit code")
	}
	// A 401 means a credential was sent and refused: having none fails
	// earlier, with its own message. So the advice must not be "log in again",
	// which is a loop, but where to look: which credential was used, and why.
	if strings.Contains(unauth.Error(), "kinit") {
		t.Errorf("the 401 message should not advise re-authenticating, got %q", unauth)
	}
	if !strings.Contains(unauth.Error(), "status") || !strings.Contains(unauth.Error(), "debug") {
		t.Errorf("the 401 message should point at status and --debug, got %q", unauth)
	}
}

func TestFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   Kind
	}{
		{http.StatusUnauthorized, KindAuth},
		{http.StatusForbidden, KindPermission},
		{http.StatusNotFound, KindNotFound},
		{http.StatusGone, KindNotFound},
		{http.StatusConflict, KindConflict},
		{http.StatusPreconditionFailed, KindConflict},
		{http.StatusLocked, KindConflict},
		{http.StatusInsufficientStorage, KindConflict},
		{http.StatusMethodNotAllowed, KindConflict},
		{http.StatusInternalServerError, KindOther},
		{http.StatusBadGateway, KindOther},
	}
	for _, tt := range tests {
		got := FromStatus(tt.status, "op", "/p", "")
		if got.Kind != tt.want {
			t.Errorf("FromStatus(%d).Kind = %v, want %v", tt.status, got.Kind, tt.want)
		}
		if got.Status != tt.status {
			t.Errorf("FromStatus(%d).Status = %d, want the original status", tt.status, got.Status)
		}
		if got.Msg == "" {
			t.Errorf("FromStatus(%d) produced an empty message", tt.status)
		}
	}
}

func TestFromStatusKeepsServerMessage(t *testing.T) {
	e := FromStatus(http.StatusForbidden, "share", "/eos/x", "sharing is disabled for this space")
	if !strings.Contains(e.Error(), "sharing is disabled") {
		t.Errorf("a server-supplied message should win over the default, got %q", e)
	}
}

func TestErrorsIsCategory(t *testing.T) {
	err := FromStatus(http.StatusNotFound, "stat", "/eos/x", "")
	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is(err, ErrNotFound) should be true")
	}
	if errors.Is(err, ErrAuth) {
		t.Error("errors.Is(err, ErrAuth) should be false for a 404")
	}

	wrapped := fmt.Errorf("listing: %w", err)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("category test should survive wrapping")
	}
}

func TestUnwrap(t *testing.T) {
	cause := errors.New("connection reset")
	err := Wrap(KindOther, "upload", "/eos/x", cause)
	if !errors.Is(err, cause) {
		t.Error("Wrap should keep the cause reachable through errors.Is")
	}
	if Wrap(KindOther, "op", "/p", nil) != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestKindOf(t *testing.T) {
	if got := KindOf(errors.New("plain")); got != KindOther {
		t.Errorf("KindOf(plain error) = %v, want KindOther", got)
	}
	if got := KindOf(Authf("expired")); got != KindAuth {
		t.Errorf("KindOf(Authf) = %v, want KindAuth", got)
	}
	if got := KindOf(nil); got != KindOther {
		t.Errorf("KindOf(nil) = %v, want KindOther", got)
	}
}

func TestConstructors(t *testing.T) {
	if got := Usagef("need %d args", 2); got.Kind != KindUsage || got.Msg != "need 2 args" {
		t.Errorf("Usagef produced %+v", got)
	}
	if got := Authf("no ticket for %s", "gdelmont"); got.Kind != KindAuth {
		t.Errorf("Authf produced %+v", got)
	}
	if got := New(KindConflict, "rm", "/eos/x", "directory not empty"); got.Kind != KindConflict {
		t.Errorf("New produced %+v", got)
	}
}
