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

package kerberos

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/test/testdata"
)

// writeKeytab materialises one of gokrb5's test keytabs on disk.
func writeKeytab(t *testing.T, dir, name, hexBytes string) string {
	t.Helper()
	b, err := hex.DecodeString(hexBytes)
	if err != nil {
		t.Fatalf("decoding the keytab fixture: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeytabValidatorLoadsAtConstruction(t *testing.T) {
	path := writeKeytab(t, t.TempDir(), "service.keytab", testdata.KEYTAB_SYSHTTP_TEST_GOKRB5)

	c := &config{Keytab: path}
	c.ApplyDefaults()
	v, err := newKeytabValidator(c)
	if err != nil {
		t.Fatalf("newKeytabValidator: %v", err)
	}
	if v.kt == nil {
		t.Error("the keytab was not loaded at construction")
	}
}

// TestKeytabIsCachedBetweenCalls: parsing a keytab on every request would be
// wasted work on a hot path.
func TestKeytabIsCachedBetweenCalls(t *testing.T) {
	path := writeKeytab(t, t.TempDir(), "service.keytab", testdata.KEYTAB_SYSHTTP_TEST_GOKRB5)

	c := &config{Keytab: path}
	c.ApplyDefaults()
	v, err := newKeytabValidator(c)
	if err != nil {
		t.Fatal(err)
	}

	first, err := v.keytab()
	if err != nil {
		t.Fatal(err)
	}
	second, err := v.keytab()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Error("the keytab was re-parsed even though the file had not changed")
	}
}

// TestKeytabIsReloadedWhenTheFileChanges is what lets a deployment rotate a
// keytab without a restart. Without it, a scheduled rotation starts rejecting
// every ticket at an arbitrary hour.
func TestKeytabIsReloadedWhenTheFileChanges(t *testing.T) {
	dir := t.TempDir()
	path := writeKeytab(t, dir, "service.keytab", testdata.KEYTAB_SYSHTTP_TEST_GOKRB5)

	c := &config{Keytab: path}
	c.ApplyDefaults()
	v, err := newKeytabValidator(c)
	if err != nil {
		t.Fatal(err)
	}
	first, err := v.keytab()
	if err != nil {
		t.Fatal(err)
	}

	// Replace the file with a different keytab and move its mtime forward, so
	// the change is visible even on a coarse-grained filesystem clock.
	replacement, err := hex.DecodeString(testdata.KEYTAB_SYSHTTP_RESDOM_GOKRB5)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	second, err := v.keytab()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Error("the keytab was not reloaded after the file changed")
	}
}

func TestKeytabValidatorReportsAMissingFile(t *testing.T) {
	c := &config{Keytab: filepath.Join(t.TempDir(), "absent.keytab")}
	c.ApplyDefaults()

	_, err := newKeytabValidator(c)
	if err == nil {
		t.Fatal("a missing keytab should be an error")
	}
	if !strings.Contains(err.Error(), "absent.keytab") {
		t.Errorf("the error should name the file, got %v", err)
	}
}

// TestValidateRejectsGarbage: the token comes straight off the wire, so the
// first thing it meets has to survive arbitrary bytes.
func TestValidateRejectsGarbage(t *testing.T) {
	path := writeKeytab(t, t.TempDir(), "service.keytab", testdata.KEYTAB_SYSHTTP_TEST_GOKRB5)

	c := &config{Keytab: path}
	c.ApplyDefaults()
	v, err := newKeytabValidator(c)
	if err != nil {
		t.Fatal(err)
	}

	for _, token := range [][]byte{
		{},
		[]byte("not a spnego token"),
		{0x60, 0x82, 0xff, 0xff},
		make([]byte, 1024),
	} {
		if _, err := v.Validate(context.Background(), token); err == nil {
			t.Errorf("token %q should have been rejected", token)
		}
	}
}

// TestDescribeStatusNamesTheAliasProblem: an SPN mismatch is by far the most
// likely deployment failure, and the raw GSS message says nothing an
// administrator can act on.
func TestDescribeStatusNamesTheAliasProblem(t *testing.T) {
	got := describeStatus(gssapi.StatusBadName, "sname does not match")
	for _, want := range []string{"DNS alias", "keytab", "reverse DNS"} {
		if !strings.Contains(got, want) {
			t.Errorf("the explanation is missing %q: %s", want, got)
		}
	}
	if !strings.Contains(got, "sname does not match") {
		t.Errorf("the mechanism's own message should be kept: %s", got)
	}
}

func TestDescribeStatus(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{gssapi.StatusBadMech, "not Kerberos"},
		{gssapi.StatusDefectiveToken, "malformed"},
		{gssapi.StatusCredentialsExpired, "expired"},
		{gssapi.StatusContextExpired, "expired"},
		{gssapi.StatusBadSig, "did not verify"},
		{gssapi.StatusDuplicateToken, "already been used"},
		{gssapi.StatusUnauthorized, "not authorized"},
	}
	for _, tt := range tests {
		if got := describeStatus(tt.code, ""); !strings.Contains(got, tt.want) {
			t.Errorf("describeStatus(%d) = %q, want it to mention %q", tt.code, got, tt.want)
		}
	}

	// An unrecognised code still has to say something, and carry the number so
	// it can be looked up.
	got := describeStatus(999999, "")
	if !strings.Contains(got, "999999") {
		t.Errorf("an unknown status should carry its code: %s", got)
	}
}
