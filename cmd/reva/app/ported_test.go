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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/cli/clitest"
)

func TestCopyTreeAndSync(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/home/.keep", nil)
	local := t.TempDir()
	for name, content := range map[string]string{"a.txt": "a", "sub/b.txt": "bb"} {
		p := filepath.Join(local, "data", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runFake(t, b, "cp", filepath.Join(local, "data"), "cb:/home/"); err == nil || !strings.Contains(err.Error(), "pass -r") {
		t.Errorf("cp of a folder without -r: %v, want to pass -r", err)
	}
	mustRun(t, b, "cp", "-r", filepath.Join(local, "data"), "cb:/home/")
	if got, _ := b.Content("/home/data/sub/b.txt"); string(got) != "bb" {
		t.Fatalf("uploaded tree: b.txt = %q", got)
	}

	back := filepath.Join(t.TempDir(), "copy")
	mustRun(t, b, "cp", "-r", "cb:/home/data", back)
	if got, _ := os.ReadFile(filepath.Join(back, "sub", "b.txt")); string(got) != "bb" {
		t.Errorf("downloaded tree: b.txt = %q", got)
	}

	if err := os.WriteFile(filepath.Join(local, "data", "c.txt"), []byte("ccc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(local, "data", "a.txt")); err != nil {
		t.Fatal(err)
	}
	mustRun(t, b, "sync", "--delete", filepath.Join(local, "data"), "cb:/home/data")
	if got, _ := b.Content("/home/data/c.txt"); string(got) != "ccc" {
		t.Errorf("sync push: c.txt = %q", got)
	}
	if _, ok := b.Content("/home/data/a.txt"); ok {
		t.Error("sync --delete kept a.txt")
	}
}

func TestCatFindDuTail(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/home/logs/job.log", []byte("one\ntwo\nthree\n"))
	b.AddFile("/home/logs/big.root", make([]byte, 2048))
	if out := mustRun(t, b, "cat", "/home/logs/job.log"); out != "one\ntwo\nthree\n" {
		t.Errorf("cat = %q", out)
	}
	if _, err := runFake(t, b, "cat", "/home/logs"); err == nil || !strings.Contains(err.Error(), "is a folder") {
		t.Errorf("cat of a folder: %v", err)
	}
	if out := mustRun(t, b, "tail", "-n", "2", "/home/logs/job.log"); out != "two\nthree\n" {
		t.Errorf("tail -n 2 = %q", out)
	}
	out := mustRun(t, b, "find", "/home", "--name", "*.root", "--size", "+1K", "--print0")
	if out != "The server cannot search: looking through the files instead...\n/home/logs/big.root\x00" &&
		!strings.HasSuffix(out, "/home/logs/big.root\x00") {
		t.Errorf("find = %q", out)
	}
	var found []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "-o", "json", "find", "/home", "--type", "f", "--name", "job")), &found); err != nil ||
		len(found) != 1 || found[0].Path != "/home/logs/job.log" {
		t.Errorf("find -o json: %v %v", found, err)
	}
	if out := mustRun(t, b, "du", "-d", "1", "/home"); !strings.Contains(out, "/home/logs") {
		t.Errorf("du -d 1 = %q", out)
	}
	if out := mustRun(t, b, "-o", "csv", "du", "--top", "1", "/home"); !strings.HasPrefix(out, "SIZE,PATH\n") {
		t.Errorf("du --top -o csv = %q", out)
	}
}

func TestVersionsDownloadAndDiff(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/home/notes.md", []byte("a\nb\n"))
	local := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(local, []byte("a\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustRun(t, b, "cp", "-f", local, "cb:/home/notes.md")
	var vs []struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "-o", "json", "versions", "list", "/home/notes.md")), &vs); err != nil || len(vs) != 1 {
		t.Fatalf("versions: %v %v", vs, err)
	}
	out := mustRun(t, b, "versions", "diff", "/home/notes.md")
	if !strings.Contains(out, "-b") || !strings.Contains(out, "+c") {
		t.Errorf("versions diff = %q", out)
	}
	dst := filepath.Join(t.TempDir(), "old.md")
	mustRun(t, b, "versions", "download", "-F", dst, "/home/notes.md", vs[0].Key)
	if got, _ := os.ReadFile(dst); string(got) != "a\nb\n" {
		t.Errorf("versions download = %q", got)
	}
	mustRun(t, b, "versions", "restore", "/home/notes.md", vs[0].Key)
	if got, _ := b.Content("/home/notes.md"); string(got) != "a\nb\n" {
		t.Errorf("after restore = %q", got)
	}
}

func TestShareAuditAndDoctor(t *testing.T) {
	b := clitest.New("alice")
	b.AddFile("/home/docs/report.pdf", []byte("x"))
	b.AddSpace(&provider.StorageSpace{Id: &provider.StorageSpaceId{OpaqueId: "home"}, Name: "home", SpaceType: "personal",
		RootInfo: &provider.ResourceInfo{Path: "/home"}})
	mustRun(t, b, "share", "create", "/home/docs", "--with", "bob", "--role", "editor")
	var grants []struct {
		Who       string `json:"who"`
		Inherited bool   `json:"inherited"`
		Role      string `json:"role"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, b, "-o", "json", "share", "audit", "/home/docs/report.pdf")), &grants); err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 || grants[0].Who != "bob" || !grants[0].Inherited || grants[0].Role != "editor" {
		t.Errorf("audit = %+v, want bob as an inherited editor", grants)
	}

	var rep struct {
		Problems int `json:"problems"`
		Checks   []struct {
			Name, Status string
		} `json:"checks"`
	}
	out, err := runFake(t, b, "-o", "json", "--token", "t", "doctor")
	if jerr := json.Unmarshal([]byte(out), &rep); jerr != nil {
		t.Fatalf("doctor: %v %v\n%s", err, jerr, out)
	}
	for _, c := range rep.Checks {
		// The clock of the fake stands still.
		if c.Status == "problem" && c.Name != "clock" {
			t.Errorf("doctor check %s failed: %s", c.Name, out)
		}
	}
	if len(rep.Checks) < 6 {
		t.Errorf("doctor ran %d checks:\n%s", len(rep.Checks), out)
	}
}
