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

package revacmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/cli"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
)

// runFake runs args on the fake backend b and returns stdout and stderr
// together.
func runFake(t *testing.T, b *clitest.Backend, args ...string) (string, error) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	rt := &cli.Runtime{}
	rt.UseBackend(b, b.Data())
	root := rootCommand(rt, new(bool))
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func mustRun(t *testing.T, b *clitest.Backend, args ...string) string {
	t.Helper()
	out, err := runFake(t, b, args...)
	if err != nil {
		t.Fatalf("reva %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestFileCommands(t *testing.T) {
	b := clitest.New("alice")
	mustRun(t, b, "mkdir", "/docs")
	mustRun(t, b, "touch", "/docs/a.txt")
	if out := mustRun(t, b, "ls", "/docs"); out != "a.txt\n" {
		t.Errorf("ls = %q, want a.txt", out)
	}
	mustRun(t, b, "mv", "/docs/a.txt", "/docs/b.txt")
	if out := mustRun(t, b, "ls", "-f", "/docs"); out != "/docs/b.txt\n" {
		t.Errorf("ls after mv = %q", out)
	}
	mustRun(t, b, "touch", "/docs/c.txt")
	if _, err := runFake(t, b, "mv", "/docs/b.txt", "/docs/c.txt"); err == nil {
		t.Error("mv onto a file that exists: no error")
	}

	mustRun(t, b, "rm", "/docs/b.txt")
	var items []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "trash", "list", "-o", "json")), &items); err != nil || len(items) != 1 {
		t.Fatalf("trash list = %v, %v: want one item", items, err)
	}
	mustRun(t, b, "trash", "restore", items[0].Key)
	if out := mustRun(t, b, "ls", "/docs"); out != "b.txt\nc.txt\n" {
		t.Errorf("ls after restore = %q", out)
	}
}

func TestCopyRoundTrip(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/docs/old.txt", nil)
	dir := t.TempDir()
	local := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	mustRun(t, b, "cp", local, "cb:/docs/")
	if got, _ := b.Content("/docs/report.txt"); string(got) != "hello" {
		t.Errorf("uploaded content = %q, want hello", got)
	}
	if err := os.WriteFile(local, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := mustRun(t, b, "cp", local, "cb:/docs/report.txt"); !strings.Contains(out, "Skipped") {
		t.Errorf("cp onto a file that exists without -f: %q, want Skipped", out)
	}
	mustRun(t, b, "cp", "-f", local, "cb:/docs/report.txt")
	if out := mustRun(t, b, "versions", "list", "/docs/report.txt"); !strings.Contains(out, "Key:") {
		t.Errorf("versions list = %q, want one version", out)
	}

	back := t.TempDir()
	mustRun(t, b, "cp", "cb:/docs/report.txt", back)
	if got, _ := os.ReadFile(filepath.Join(back, "report.txt")); string(got) != "changed" {
		t.Errorf("downloaded content = %q, want changed", got)
	}
}

func TestShareCommands(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/docs/a.txt", []byte("x"))
	var created []struct {
		ID struct {
			OpaqueID string `json:"opaqueId"`
		} `json:"id"`
	}
	out := mustRun(t, b, "share", "create", "--with", "bob", "--with", "carol", "/docs/a.txt", "-o", "json")
	if err := json.Unmarshal([]byte(out), &created); err != nil || len(created) != 2 {
		t.Fatalf("share create = %s, %v: want two shares", out, err)
	}
	if out := mustRun(t, b, "share", "list", "/docs/a.txt"); !strings.Contains(out, "bob") || !strings.Contains(out, "carol") {
		t.Errorf("share list = %q, want bob and carol", out)
	}
	mustRun(t, b, "share", "update", created[0].ID.OpaqueID, "--role", "editor")
	mustRun(t, b, "share", "remove", created[1].ID.OpaqueID)
	if out := mustRun(t, b, "share", "list"); strings.Contains(out, "carol") {
		t.Errorf("share list after remove = %q", out)
	}

	out = mustRun(t, b, "link", "create", "/docs/a.txt", "--description", "for the team")
	if !strings.Contains(out, "for the team") {
		t.Errorf("link create = %q", out)
	}
	if out := mustRun(t, b, "link", "list"); !strings.Contains(out, "for the team") {
		t.Errorf("link list = %q", out)
	}
}

func TestTokenCommands(t *testing.T) {
	b := clitest.New("alice")
	mustRun(t, b, "token", "create", "--all", "--label", "nightly")
	if out := mustRun(t, b, "token", "list"); !strings.Contains(out, "nightly") {
		t.Errorf("token list = %q", out)
	}
}

// A call that the transport does not have fails with a clear message, after
// the command started: exit code 1.
func TestNotAvailable(t *testing.T) {
	b := clitest.New("alice")
	for _, args := range [][]string{
		{"preferences", "get", "ns", "key"},
		{"ocm", "invite", "list"},
		{"ocm", "transfer", "list"},
	} {
		_, err := runFake(t, b, args...)
		if !errors.Is(err, backend.ErrNotAvailable) || !strings.Contains(err.Error(), "not available over fake") {
			t.Errorf("reva %s: err = %v, want not available over fake", strings.Join(args, " "), err)
		}
	}
}
