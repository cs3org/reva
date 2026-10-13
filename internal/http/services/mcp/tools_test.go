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

package mcp

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	link "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/permissions"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestToolList(t *testing.T) {
	h := newHarness(t, map[string]any{"disabled_tools": []string{toolMove}})
	readOnly := []string{toolGetLabels, toolListFolder, toolListShares, toolListSpaces, toolListVersions, toolReadFile, toolStat, toolWebLink}
	cases := map[string][]string{
		"ro":     readOnly,
		"rw":     slices.Sorted(slices.Values(append(slices.Clone(readOnly), toolCreateFolder, toolSetLabels, toolWriteFile))),
		"nolist": nil,
	}
	for bearer, want := range cases {
		if got := toolNames(t, h.connect(bearer)); !slices.Equal(got, want) {
			t.Errorf("%s: tools %v, want %v", bearer, got, want)
		}
	}
	all := toolNames(t, h.connect("all"))
	if len(all) != 18 || slices.Contains(all, toolMove) || slices.Contains(all, "search_files") {
		t.Errorf("full catalogue, move disabled: %v", all)
	}
}

func TestToolSchemas(t *testing.T) {
	h := newHarness(t, nil)
	res, err := h.connect("all").ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Description == "" || tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Errorf("%s: description, input and output schema are needed", tool.Name)
		}
		if tool.Name == toolWriteFile {
			props := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
			if enum := props["encoding"].(map[string]any)["enum"]; fmt.Sprint(enum) != "[utf8 base64]" {
				t.Errorf("write_file encoding: %v", enum)
			}
		}
		if ro := tool.Annotations.ReadOnlyHint; ro != !slices.Contains([]string{
			toolWriteFile, toolCreateFolder, toolMove, toolDelete, toolRestoreVersion, toolRestoreFromTrash,
			toolSetLabels, toolCreateShare, toolCreatePublicLink, toolRemoveShare,
		}, tool.Name) {
			t.Errorf("%s: read only hint %v", tool.Name, ro)
		}
	}
}

// Every handler checks the permissions again, whatever the client was told.
func TestPermissionGating(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/a.txt", "a")
	ro := h.connect("ro")
	calls := map[string]map[string]any{
		toolWriteFile:        {"path": home + "/b.txt", "content": "b"},
		toolCreateFolder:     {"path": home + "/d"},
		toolMove:             {"source": home + "/a.txt", "destination": home + "/c.txt"},
		toolDelete:           {"path": home + "/a.txt"},
		toolRestoreVersion:   {"path": home + "/a.txt", "version_key": "1"},
		toolListTrash:        {},
		toolRestoreFromTrash: {"key": "x"},
		toolSetLabels:        {"path": home + "/a.txt", "labels": []string{"x"}},
		toolCreateShare:      {"path": home + "/a.txt", "grantee": "marie"},
		toolCreatePublicLink: {"path": home + "/a.txt"},
		toolRemoveShare:      {"share_id": "1"},
	}
	for name, args := range calls {
		if e := failure(t, callTool(t, ro, name, args)); e.Code != codeNotPermittedByAP {
			t.Errorf("%s with a read-only app password: %+v", name, e)
		}
	}
	if e := failure(t, callTool(t, h.connect("nolist"), toolReadFile, map[string]any{"path": home + "/a.txt"})); e.Code != codeNotPermittedByAP || !strings.Contains(e.Message, "files:list") {
		t.Errorf("read_file without files:list: %+v", e)
	}
	if m := h.gw.Methods(); len(m) != 0 {
		t.Errorf("refused calls must not reach the gateway: %v", m)
	}
}

func TestRoots(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Mkdir(home + "/thesis")
	h.gw.Put(home+"/thesis/ch1.md", "# One")
	h.gw.Put(home+"/private.txt", "secret")
	thesis := h.gw.Put(home+"/thesis/ch2.md", "# Two")
	private := h.gw.Put(home+"/private/x.txt", "x")
	// The interceptor enforces the roots on every call; here, on what is outside.
	h.gw.Deny(home + "/private")
	h.gw.Deny(home + "/private.txt")
	h.gw.received = []*collaboration.ReceivedShare{
		{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "s1"}, ResourceId: thesis.Id, Owner: einstein.Id}},
		{Share: &collaboration.Share{Id: &collaboration.ShareId{OpaqueId: "s2"}, ResourceId: private.Id, Owner: einstein.Id}},
	}
	cs := h.connect("roots")

	spaces := decode[listSpacesOut](t, callTool(t, cs, toolListSpaces, nil))
	if len(spaces.Spaces) != 0 || !slices.Equal(spaces.Roots, []string{home + "/thesis"}) {
		t.Errorf("list_spaces: spaces outside the roots must be left out, roots shown: %+v", spaces)
	}
	if out := decode[listFolderOut](t, callTool(t, cs, toolListFolder, map[string]any{"path": home + "/thesis"})); len(out.Entries) != 2 {
		t.Errorf("list_folder inside the roots: %+v", out)
	}
	outside := []struct {
		tool string
		args map[string]any
	}{
		{toolStat, map[string]any{"path": home + "/private.txt"}},
		{toolListFolder, map[string]any{"path": home}},
		{toolReadFile, map[string]any{"path": home + "/thesis/../private.txt"}},
		{toolMove, map[string]any{"source": home + "/thesis/ch1.md", "destination": home + "/ch1.md"}},
		{toolWriteFile, map[string]any{"path": home + "/new.txt", "content": "x"}},
		{toolCreateShare, map[string]any{"path": home + "/private.txt", "grantee": "marie"}},
	}
	for _, c := range outside {
		e := failure(t, callTool(t, cs, c.tool, c.args))
		if e.Code != codeOutsideRoots || !strings.Contains(e.Message, home+"/thesis") {
			t.Errorf("%s %v: %+v", c.tool, c.args, e)
		}
	}

	shares := decode[listSharesOut](t, callTool(t, cs, toolListShares, nil))
	if len(shares.Shares) != 1 || shares.Shares[0].ID != "s1" || shares.Shares[0].Path != home+"/thesis/ch2.md" {
		t.Errorf("list_shares must leave out what is outside the roots: %+v", shares)
	}

	// The recycle bin of the home is listed through the root inside it, and
	// shows only what was deleted under the roots.
	callTool(t, h.connect("all"), toolDelete, map[string]any{"path": home + "/thesis/ch1.md"})
	h.gw.mu.Lock()
	h.gw.trash = append(h.gw.trash, &provider.RecycleItem{Key: "elsewhere", Ref: &provider.Reference{Path: home + "/private/y.txt"}})
	h.gw.mu.Unlock()
	trash := decode[listTrashOut](t, callTool(t, cs, toolListTrash, nil))
	if len(trash.Items) != 1 || trash.Items[0].Path != home+"/thesis/ch1.md" {
		t.Fatalf("list_trash: %+v", trash)
	}
	// A key naming an item outside the roots is refused, whatever it claims.
	forged := trashKey{Ref: home + "/thesis", Key: "elsewhere"}.encode()
	if e := failure(t, callTool(t, cs, toolRestoreFromTrash, map[string]any{"key": forged})); e.Code != codeOutsideRoots {
		t.Errorf("restoring an item outside the roots: %+v", e)
	}
	if out := decode[restoreFromTrashOut](t, callTool(t, cs, toolRestoreFromTrash, map[string]any{"key": trash.Items[0].Key})); out.Path != home+"/thesis/ch1.md" {
		t.Errorf("restore_from_trash: %+v", out)
	}
}

func TestListFolderPages(t *testing.T) {
	h := newHarness(t, map[string]any{"list_max_entries": 3})
	h.gw.Mkdir(home + "/docs")
	var want []string
	for i := range 7 {
		p := fmt.Sprintf("%s/docs/f%d.txt", home, i)
		h.gw.Put(p, "x")
		want = append(want, p)
	}
	cs := h.connect("ro")
	var got []string
	cursor := ""
	pages := 0
	for {
		out := decode[listFolderOut](t, callTool(t, cs, toolListFolder, map[string]any{"path": home + "/docs", "cursor": cursor, "limit": 100}))
		pages++
		if len(out.Entries) > 3 {
			t.Fatalf("page of %d entries, more than list_max_entries", len(out.Entries))
		}
		for _, e := range out.Entries {
			got = append(got, e.Path)
			if e.Type != "file" || e.WebURL != "https://cernbox.example.org/files/spaces"+e.Path || e.Etag == "" || e.ID == "" || e.Mtime == "" {
				t.Errorf("entry %+v", e)
			}
		}
		if out.Folder.Path != home+"/docs" || out.Folder.Type != "folder" {
			t.Errorf("folder %+v", out.Folder)
		}
		if out.NextCursor == "" {
			break
		}
		cursor = out.NextCursor
		// A file that appears after the first page does not shift the pages.
		if pages == 1 {
			h.gw.Put(home+"/docs/a-new.txt", "x")
		}
	}
	if pages != 3 || !slices.Equal(got, want) {
		t.Errorf("%d pages, entries %v", pages, got)
	}
	if out := decode[listFolderOut](t, callTool(t, cs, toolListFolder, map[string]any{"path": home + "/docs", "limit": 2})); len(out.Entries) != 2 {
		t.Errorf("limit: %d entries", len(out.Entries))
	}
	if e := failure(t, callTool(t, cs, toolListFolder, map[string]any{"path": home + "/docs", "cursor": "!!"})); e.Code != codeInvalidArgument {
		t.Errorf("bad cursor: %+v", e)
	}
	if e := failure(t, callTool(t, cs, toolListFolder, map[string]any{"path": home + "/docs/f1.txt"})); e.Code != codeInvalidArgument {
		t.Errorf("listing a file: %+v", e)
	}
}

func TestReadFile(t *testing.T) {
	h := newHarness(t, map[string]any{"max_read_bytes": 8, "max_image_bytes": 16})
	h.gw.Put(home+"/short.md", "# hi")
	h.gw.Put(home+"/long.txt", "0123456789abcdefghij")
	// é is two bytes, at bytes 7 and 8: a part must not cut it.
	h.gw.Put(home+"/accents.txt", "abcdefgé tail")
	h.gw.Put(home+"/plot.png", "\x89PNG fake image")
	h.gw.Put(home+"/huge.png", strings.Repeat("x", 17))
	h.gw.Put(home+"/paper.pdf", "%PDF-1.7")
	h.gw.Put(home+"/Makefile", "all:\n\tgo")
	h.gw.Put(home+"/blob", "\x00\x01\x02binary")
	h.gw.Put(home+"/empty.txt", "")
	cs := h.connect("ro")
	read := func(p string, args ...any) *sdk.CallToolResult {
		a := map[string]any{"path": p}
		for i := 0; i < len(args); i += 2 {
			a[args[i].(string)] = args[i+1]
		}
		return callTool(t, cs, toolReadFile, a)
	}

	if out := decode[readFileOut](t, read(home+"/short.md")); out.Content != "# hi" || out.Truncated || out.Size != 4 || out.ReturnedBytes != 4 {
		t.Errorf("short file: %+v", out)
	}

	var parts []string
	offset := int64(0)
	for range 5 {
		out := decode[readFileOut](t, read(home+"/long.txt", "offset", offset))
		parts = append(parts, out.Content)
		if !out.Truncated {
			break
		}
		if out.NextOffset != offset+out.ReturnedBytes {
			t.Fatalf("next_offset %d after %d+%d", out.NextOffset, offset, out.ReturnedBytes)
		}
		offset = out.NextOffset
	}
	if strings.Join(parts, "|") != "01234567|89abcdef|ghij" {
		t.Errorf("paged read: %q", parts)
	}
	if out := decode[readFileOut](t, read(home+"/long.txt", "offset", 2, "length", 3)); out.Content != "234" || !out.Truncated || out.NextOffset != 5 {
		t.Errorf("length: %+v", out)
	}
	if out := decode[readFileOut](t, read(home+"/long.txt", "offset", 100)); out.Content != "" || out.Truncated || out.ReturnedBytes != 0 {
		t.Errorf("offset past the end: %+v", out)
	}

	first := decode[readFileOut](t, read(home+"/accents.txt"))
	if first.Content != "abcdefg" || first.NextOffset != 7 {
		t.Errorf("a part ends before a rune it would cut: %+v", first)
	}
	if second := decode[readFileOut](t, read(home+"/accents.txt", "offset", 7)); !strings.HasPrefix(second.Content, "é") {
		t.Errorf("the next part starts with the rune: %+v", second)
	}

	res := read(home + "/plot.png")
	img, ok := res.Content[0].(*sdk.ImageContent)
	if res.IsError || !ok || img.MIMEType != "image/png" || string(img.Data) != "\x89PNG fake image" {
		t.Errorf("image: %+v", res.Content)
	}
	if e := failure(t, read(home+"/huge.png")); e.Code != codeTooLarge || e.WebURL == "" {
		t.Errorf("image over max_image_bytes: %+v", e)
	}
	if e := failure(t, read(home+"/paper.pdf")); e.Code != codeUnsupportedType || e.WebURL != "https://cernbox.example.org/files/spaces"+home+"/paper.pdf" {
		t.Errorf("pdf: %+v", e)
	}
	if out := decode[readFileOut](t, read(home+"/Makefile")); out.Content != "all:\n\tgo" {
		t.Errorf("text of an unknown type: %+v", out)
	}
	if e := failure(t, read(home+"/blob")); e.Code != codeUnsupportedType {
		t.Errorf("binary of an unknown type: %+v", e)
	}
	if out := decode[readFileOut](t, read(home+"/empty.txt")); out.Content != "" || out.Truncated {
		t.Errorf("empty file: %+v", out)
	}
	if e := failure(t, read(home)); e.Code != codeInvalidArgument {
		t.Errorf("folder: %+v", e)
	}
	if e := failure(t, read(home+"/long.txt", "offset", -1)); e.Code != codeInvalidArgument {
		t.Errorf("negative offset: %+v", e)
	}
}

func TestWriteFile(t *testing.T) {
	h := newHarness(t, map[string]any{"max_write_bytes": 10})
	cs := h.connect("rw")
	write := func(args map[string]any) *sdk.CallToolResult { return callTool(t, cs, toolWriteFile, args) }

	created := decode[entry](t, write(map[string]any{"path": home + "/new.txt", "content": "v1"}))
	if created.Size != 2 || created.Etag == "" || created.Type != "file" {
		t.Fatalf("create: %+v", created)
	}
	updated := decode[entry](t, write(map[string]any{"path": home + "/new.txt", "content": "v2!", "if_match": created.Etag}))
	if updated.Size != 3 || updated.Etag == created.Etag {
		t.Fatalf("update with the current etag: %+v", updated)
	}
	if e := failure(t, write(map[string]any{"path": home + "/new.txt", "content": "v3", "if_match": created.Etag})); e.Code != codePreconditionFailed || !strings.Contains(e.Message, updated.Etag) {
		t.Errorf("update with a stale etag: %+v", e)
	}
	if e := failure(t, write(map[string]any{"path": home + "/missing.txt", "content": "x", "if_match": `"1"`})); e.Code != codePreconditionFailed {
		t.Errorf("if_match on a missing file: %+v", e)
	}
	bin := decode[entry](t, write(map[string]any{"path": home + "/bin", "content": base64.StdEncoding.EncodeToString([]byte{0, 1, 2}), "encoding": "base64"}))
	if bin.Size != 3 {
		t.Errorf("base64: %+v", bin)
	}
	if e := failure(t, write(map[string]any{"path": home + "/bin", "content": "%%%", "encoding": "base64"})); e.Code != codeInvalidArgument {
		t.Errorf("bad base64: %+v", e)
	}
	if res := write(map[string]any{"path": home + "/bin", "content": "x", "encoding": "latin1"}); !res.IsError {
		t.Error("an encoding outside the enum must be refused")
	}
	if e := failure(t, write(map[string]any{"path": home + "/big.txt", "content": strings.Repeat("x", 11)})); e.Code != codeTooLarge {
		t.Errorf("over max_write_bytes: %+v", e)
	}
	if e := failure(t, write(map[string]any{"path": home + "/no/such/dir/x.txt", "content": "x"})); e.Code != codeNotFound {
		t.Errorf("missing folder: %+v", e)
	}
	h.gw.mu.Lock()
	content := string(h.gw.files[home+"/new.txt"].content)
	h.gw.mu.Unlock()
	if content != "v2!" {
		t.Errorf("stored %q", content)
	}
}

func TestErrors(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/locked/a.txt", "a")
	h.gw.Deny(home + "/locked")
	cs := h.connect("ro")
	cases := []struct {
		path, code string
	}{
		{home + "/missing.txt", codeNotFound},
		// A permission denied from the auth interceptor of a service.
		{home + "/locked/a.txt", codePermissionDenied},
		{"notes.txt", codeInvalidArgument},
		{"", codeInvalidArgument},
	}
	for _, c := range cases {
		if e := failure(t, callTool(t, cs, toolStat, map[string]any{"path": c.path})); e.Code != c.code {
			t.Errorf("stat %q: %+v, want %s", c.path, e, c.code)
		}
	}
	if e := failure(t, callTool(t, cs, toolStat, map[string]any{"path": "notes.txt"})); !strings.Contains(e.Message, "list_spaces") {
		t.Errorf("a relative path must point at list_spaces: %q", e.Message)
	}
}

func TestSpacesAndLinks(t *testing.T) {
	h := newHarness(t, nil)
	cs := h.connect("ro")
	out := decode[listSpacesOut](t, callTool(t, cs, toolListSpaces, nil))
	if len(out.Spaces) != 2 || out.Spaces[0].Path != home || out.Spaces[1].Type != "project" || out.Roots != nil {
		t.Errorf("list_spaces: %+v", out)
	}
	h.gw.Put(home+"/a b#c.txt", "x")
	if l := decode[webLinkOut](t, callTool(t, cs, toolWebLink, map[string]any{"path": home + "/a b#c.txt"})); l.WebURL != "https://cernbox.example.org/files/spaces/eos/user/e/einstein/a%20b%23c.txt" {
		t.Errorf("web_link escapes the path: %+v", l)
	}
}

func TestFolderMoveDeleteTrash(t *testing.T) {
	h := newHarness(t, nil)
	cs := h.connect("all")
	if out := decode[entry](t, callTool(t, cs, toolCreateFolder, map[string]any{"path": home + "/reports"})); out.Type != "folder" {
		t.Errorf("create_folder: %+v", out)
	}
	if e := failure(t, callTool(t, cs, toolCreateFolder, map[string]any{"path": home + "/reports"})); e.Code != codePreconditionFailed {
		t.Errorf("existing folder: %+v", e)
	}
	f := h.gw.Put(home+"/reports/q1.txt", "q1")
	if out := decode[entry](t, callTool(t, cs, toolMove, map[string]any{"source": home + "/reports/q1.txt", "destination": home + "/reports/2026-q1.txt"})); out.Path != home+"/reports/2026-q1.txt" {
		t.Errorf("move: %+v", out)
	}
	if e := failure(t, callTool(t, cs, toolDelete, map[string]any{"path": home + "/reports/2026-q1.txt", "if_match": `"stale"`})); e.Code != codePreconditionFailed {
		t.Errorf("delete with a stale etag: %+v", e)
	}
	if out := decode[deleteOut](t, callTool(t, cs, toolDelete, map[string]any{"path": home + "/reports/2026-q1.txt", "if_match": f.Etag})); !out.Deleted {
		t.Errorf("delete: %+v", out)
	}
	trash := decode[listTrashOut](t, callTool(t, cs, toolListTrash, map[string]any{"space": home}))
	if len(trash.Items) != 1 || trash.Items[0].Path != home+"/reports/2026-q1.txt" || trash.Items[0].Deleted == "" {
		t.Fatalf("list_trash: %+v", trash)
	}
	// The storage restores where the item was deleted from.
	out := decode[restoreFromTrashOut](t, callTool(t, cs, toolRestoreFromTrash, map[string]any{"key": trash.Items[0].Key, "destination": home + "/elsewhere.txt"}))
	if out.Path != home+"/reports/2026-q1.txt" {
		t.Errorf("restore_from_trash reports where the item is: %+v", out)
	}
	if e := failure(t, callTool(t, cs, toolRestoreFromTrash, map[string]any{"key": trash.Items[0].Key})); e.Code != codeNotFound {
		t.Errorf("restoring twice: %+v", e)
	}
	if e := failure(t, callTool(t, cs, toolRestoreFromTrash, map[string]any{"key": "garbage"})); e.Code != codeInvalidArgument {
		t.Errorf("bad key: %+v", e)
	}
}

func TestTrashPages(t *testing.T) {
	h := newHarness(t, map[string]any{"list_max_entries": 2})
	cs := h.connect("all")
	for i := range 5 {
		p := fmt.Sprintf("%s/f%d", home, i)
		h.gw.Put(p, "x")
		callTool(t, cs, toolDelete, map[string]any{"path": p})
	}
	var got []string
	cursor := ""
	for {
		out := decode[listTrashOut](t, callTool(t, cs, toolListTrash, map[string]any{"cursor": cursor}))
		for _, it := range out.Items {
			got = append(got, it.Path)
		}
		if cursor = out.NextCursor; cursor == "" {
			break
		}
	}
	want := []string{home + "/f4", home + "/f3", home + "/f2", home + "/f1", home + "/f0"}
	if !slices.Equal(got, want) {
		t.Errorf("newest first, all pages: %v", got)
	}
}

func TestVersions(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/a.txt", "v1")
	h.gw.Put(home+"/a.txt", "v2")
	h.gw.Put(home+"/a.txt", "v3")
	cs := h.connect("all")
	out := decode[listVersionsOut](t, callTool(t, cs, toolListVersions, map[string]any{"path": home + "/a.txt"}))
	if len(out.Versions) != 2 || out.Versions[0].Mtime < out.Versions[1].Mtime {
		t.Fatalf("list_versions, newest first: %+v", out)
	}
	oldest := out.Versions[1]
	if e := decode[entry](t, callTool(t, cs, toolRestoreVersion, map[string]any{"path": home + "/a.txt", "version_key": oldest.Key})); e.Size != 2 {
		t.Errorf("restore_version: %+v", e)
	}
	if r := decode[readFileOut](t, callTool(t, cs, toolReadFile, map[string]any{"path": home + "/a.txt"})); r.Content != "v1" {
		t.Errorf("restored content %q", r.Content)
	}
	if e := failure(t, callTool(t, cs, toolRestoreVersion, map[string]any{"path": home + "/a.txt", "version_key": "nope"})); e.Code != codeNotFound {
		t.Errorf("unknown version: %+v", e)
	}
}

func TestLabels(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/a.txt", "a")
	cs := h.connect("rw")
	set := func(ls ...string) labelsOut {
		return decode[labelsOut](t, callTool(t, cs, toolSetLabels, map[string]any{"path": home + "/a.txt", "labels": ls}))
	}
	if out := set("todo", "physics"); !slices.Equal(out.Labels, []string{"physics", "todo"}) {
		t.Errorf("set: %+v", out)
	}
	if out := set("physics", "done"); !slices.Equal(out.Labels, []string{"done", "physics"}) {
		t.Errorf("replace: %+v", out)
	}
	if out := decode[labelsOut](t, callTool(t, cs, toolGetLabels, map[string]any{"path": home + "/a.txt"})); !slices.Equal(out.Labels, []string{"done", "physics"}) {
		t.Errorf("get: %+v", out)
	}
	if out := set(); len(out.Labels) != 0 {
		t.Errorf("clear: %+v", out)
	}
}

func TestShares(t *testing.T) {
	h := newHarness(t, nil)
	doc := h.gw.Put(home+"/doc.txt", "d")
	h.gw.Mkdir(home + "/drop")
	marie := h.gw.users["marie"]
	h.gw.received = []*collaboration.ReceivedShare{{
		Share: &collaboration.Share{
			Id: &collaboration.ShareId{OpaqueId: "r1"}, ResourceId: h.gw.Put(project+"/plan.md", "p").Id, Owner: marie.Id,
			Grantee:     &provider.Grantee{Type: provider.GranteeType_GRANTEE_TYPE_USER, Id: &provider.Grantee_UserId{UserId: einstein.Id}},
			Permissions: &collaboration.SharePermissions{Permissions: permissions.NewEditorRole().CS3ResourcePermissions()},
		},
		State: collaboration.ShareState_SHARE_STATE_ACCEPTED,
	}}
	cs := h.connect("all")

	in := decode[listSharesOut](t, callTool(t, cs, toolListShares, nil))
	if len(in.Shares) != 1 || in.Shares[0].Owner != "marie" || in.Shares[0].State != "accepted" || in.Shares[0].Role != "editor" || in.Shares[0].Path != project+"/plan.md" {
		t.Errorf("incoming: %+v", in)
	}

	user := decode[shareEntry](t, callTool(t, cs, toolCreateShare, map[string]any{"path": home + "/doc.txt", "grantee": "marie"}))
	if user.Kind != "user" || user.Grantee != "marie" || user.Role != "viewer" || user.Path != home+"/doc.txt" {
		t.Errorf("create_share: %+v", user)
	}
	group := decode[shareEntry](t, callTool(t, cs, toolCreateShare, map[string]any{"path": home + "/doc.txt", "grantee": "group:physicists", "role": "editor"}))
	if group.Kind != "group" || group.Grantee != "group:physicists" || group.Role != "editor" {
		t.Errorf("create_share with a group: %+v", group)
	}
	if e := failure(t, callTool(t, cs, toolCreateShare, map[string]any{"path": home + "/doc.txt", "grantee": "nobody"})); e.Code != codeNotFound {
		t.Errorf("unknown user: %+v", e)
	}

	expiry := time.Now().AddDate(0, 1, 0).Format(time.DateOnly)
	pub := decode[shareEntry](t, callTool(t, cs, toolCreatePublicLink, map[string]any{"path": home + "/doc.txt", "expiry": expiry, "password": "s3cret"}))
	if pub.Kind != "link" || !strings.HasPrefix(pub.ID, linkIDPrefix) || !strings.HasPrefix(pub.URL, "https://cernbox.example.org/s/tok") ||
		!pub.PasswordProtected || !strings.HasPrefix(pub.Expiry, expiry) || pub.Role != "viewer" {
		t.Errorf("create_public_link: %+v", pub)
	}
	if e := failure(t, callTool(t, cs, toolCreatePublicLink, map[string]any{"path": home + "/doc.txt", "role": "uploader"})); e.Code != codeInvalidArgument {
		t.Errorf("uploader link to a file: %+v", e)
	}
	if up := decode[shareEntry](t, callTool(t, cs, toolCreatePublicLink, map[string]any{"path": home + "/drop", "role": "uploader"})); up.Role != "uploader" {
		t.Errorf("uploader link: %+v", up)
	}
	if e := failure(t, callTool(t, cs, toolCreatePublicLink, map[string]any{"path": home + "/doc.txt", "expiry": "2001-01-01"})); e.Code != codeInvalidArgument {
		t.Errorf("expiry in the past: %+v", e)
	}

	out := decode[listSharesOut](t, callTool(t, cs, toolListShares, map[string]any{"direction": "outgoing"}))
	if len(out.Shares) != 4 {
		t.Fatalf("outgoing: %+v", out)
	}
	for _, id := range []string{user.ID, pub.ID} {
		if r := decode[removeShareOut](t, callTool(t, cs, toolRemoveShare, map[string]any{"share_id": id})); !r.Removed {
			t.Errorf("remove %s: %+v", id, r)
		}
	}
	if e := failure(t, callTool(t, cs, toolRemoveShare, map[string]any{"share_id": user.ID})); e.Code != codeNotFound {
		t.Errorf("removing twice: %+v", e)
	}
	h.gw.mu.Lock()
	defer h.gw.mu.Unlock()
	if len(h.gw.shares) != 1 || len(h.gw.links) != 1 || h.gw.shares[0].ResourceId.OpaqueId != doc.Id.OpaqueId {
		t.Errorf("left: %v shares, %v links", h.gw.shares, h.gw.links)
	}
}

func TestRemoveShareRoots(t *testing.T) {
	h := newHarness(t, nil)
	inside := h.gw.Put(home+"/thesis/a.txt", "a")
	outside := h.gw.Put(home+"/b.txt", "b")
	h.gw.Deny(home + "/b.txt")
	h.gw.shares = []*collaboration.Share{
		{Id: &collaboration.ShareId{OpaqueId: "in"}, ResourceId: inside.Id},
		{Id: &collaboration.ShareId{OpaqueId: "out"}, ResourceId: outside.Id},
	}
	h.gw.links = []*link.PublicShare{{Id: &link.PublicShareId{OpaqueId: "9"}, ResourceId: outside.Id}}
	cs := h.connect("roots")
	if r := decode[removeShareOut](t, callTool(t, cs, toolRemoveShare, map[string]any{"share_id": "in"})); !r.Removed {
		t.Errorf("share inside the roots: %+v", r)
	}
	for _, id := range []string{"out", linkIDPrefix + "9"} {
		if e := failure(t, callTool(t, cs, toolRemoveShare, map[string]any{"share_id": id})); e.Code != codePermissionDenied && e.Code != codeOutsideRoots {
			t.Errorf("%s outside the roots: %+v", id, e)
		}
	}
	if slices.Contains(h.gw.Methods(), "RemovePublicShare") || len(h.gw.shares) != 1 {
		t.Error("nothing outside the roots may be removed")
	}
}
