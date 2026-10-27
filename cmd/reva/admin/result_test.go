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
	"encoding/json"
	"strings"
	"testing"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

var logsSpec = &adminpb.InvocationSpec{Name: "logs", Kind: "readonly", Streaming: true}

// lines squeezes the spaces of each line, so that a test does not depend on
// the column widths.
func lines(s string) []string {
	var out []string
	for l := range strings.SplitSeq(s, "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestInvokeListIsATable(t *testing.T) {
	f := &fakeAdmin{specs: []*adminpb.InvocationSpec{logsSpec}, results: []*adminpb.NodeResult{
		{Node: "a:1/svc", ResultJson: `{"truncated":false,"entries":[{"time":"t1","level":"info","ctx":{"user":"alice"}}]}`},
		{Node: "b:1/svc", ResultJson: `{"truncated":true,"entries":[{"time":"t2","level":"warn","extra":[1,2]}]}`},
	}}
	out, _, err := run(t, f, "invoke", "svc", "logs")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(lines(out), "\n")
	for _, want := range []string{
		"| NODE | TIME | LEVEL | CTX.USER | EXTRA |",
		"| a:1/svc | t1 | info | alice | |",
		"| b:1/svc | t2 | warn | | 1,2 |",
		"a:1/svc truncated: false",
		"b:1/svc truncated: true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}

func TestInvokeCSV(t *testing.T) {
	f := &fakeAdmin{specs: []*adminpb.InvocationSpec{logsSpec}, results: []*adminpb.NodeResult{
		{Node: "a:1/svc", ResultJson: `{"truncated":false,"entries":[{"time":"t1","level":"info","ctx":{"user":"alice, bob"}}]}`},
	}}
	out, _, err := run(t, f, "-o", "csv", "invoke", "svc", "logs")
	if err != nil {
		t.Fatal(err)
	}
	want := "time,level,ctx.user\nt1,info,\"alice, bob\"\nkey,value\ntruncated,false\n"
	if out != want {
		t.Errorf("csv:\n%s\nwant:\n%s", out, want)
	}
}

func TestInvokeOneNodeAndATopLevelArray(t *testing.T) {
	f := &fakeAdmin{results: []*adminpb.NodeResult{{Node: "a:1/svc", ResultJson: `[{"b":1},{"a":2}]`}}}
	out, _, err := run(t, f, "invoke", "a:1/svc", "op")
	if err != nil {
		t.Fatal(err)
	}
	if got := lines(out); len(got) != 6 || got[1] != "| B | A |" || got[3] != "| 1 | |" {
		t.Errorf("table:\n%s", out)
	}
}

func TestInvokeObjectIsKeyValueLines(t *testing.T) {
	f := &fakeAdmin{results: []*adminpb.NodeResult{{Node: "a:1/svc", ResultJson: `{"version":"3.1","build":{"go":"1.27","date":null},"size":1e3}`}}}
	out, _, err := run(t, f, "invoke", "svc", "version")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(lines(out), "|"), "version: 3.1|build.go: 1.27|build.date:|size: 1e3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInvokeNodeErrorFails(t *testing.T) {
	f := &fakeAdmin{results: []*adminpb.NodeResult{
		{Node: "a:1/svc", ResultJson: `{"ok":true}`},
		{Node: "b:1/svc", Error: "boom", Code: 13},
	}}
	out, errOut, err := run(t, f, "invoke", "svc", "op")
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("err = %v, want a failure of 1 of 2 instances", err)
	}
	if strings.TrimSpace(errOut) != "b:1/svc: boom" || !strings.Contains(strings.Join(lines(out), "|"), "a:1/svc ok: true") {
		t.Errorf("stdout:\n%s\nstderr:\n%s", out, errOut)
	}
}

func TestInvokeJSONIsOneDocument(t *testing.T) {
	f := &fakeAdmin{results: []*adminpb.NodeResult{
		{Node: "a:1/svc", ResultJson: `{"n":1}`},
		{Node: "b:1/svc", Error: "boom"},
	}}
	out, _, err := run(t, f, "-o", "json", "invoke", "svc", "op")
	if err == nil {
		t.Error("want an error for the failed node")
	}
	var got []struct {
		Node   string
		Result map[string]any
		Error  string
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out)
	}
	if len(got) != 2 || got[0].Result["n"] != 1.0 || got[1].Error != "boom" || got[1].Result != nil {
		t.Errorf("got %+v", got)
	}
}

func TestInvokeStreamsFromTheCatalogue(t *testing.T) {
	spec := &adminpb.InvocationSpec{Name: "tail", Kind: "readonly", Streaming: true, StreamOnly: true}
	stream := []*adminpb.InvokeStreamResponse{
		{Node: "a:1/svc", ResultJson: `{"time":"t1","message":"started"}`},
		{Node: "b:1/svc", ResultJson: `{"time":"t2","message":"x","new":"not shown"}`},
		{Node: "b:1/svc", Error: "gone"},
	}
	f := &fakeAdmin{specs: []*adminpb.InvocationSpec{spec}, stream: stream}
	out, errOut, err := run(t, f, "invoke", "svc", "tail")
	if err == nil {
		t.Error("want an error for the failed node")
	}
	if got, want := strings.Join(lines(out), "|"), "NODE TIME MESSAGE|a:1/svc t1 started|b:1/svc t2 x"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if strings.TrimSpace(errOut) != "b:1/svc: gone" {
		t.Errorf("stderr: %q", errOut)
	}

	out, _, _ = run(t, f, "-o", "json", "invoke", "svc", "tail")
	got := strings.Split(strings.TrimSpace(out), "\n")
	if len(got) != 3 {
		t.Fatalf("want one JSON document per line:\n%s", out)
	}
	for _, l := range got {
		var v struct {
			Node   string
			Result json.RawMessage
		}
		if err := json.Unmarshal([]byte(l), &v); err != nil || v.Node == "" {
			t.Errorf("line %q: %v", l, err)
		}
	}
}

// An invocation with both forms, such as logs, streams only with --stream.
func TestInvokeStreamFlag(t *testing.T) {
	f := &fakeAdmin{
		specs:   []*adminpb.InvocationSpec{logsSpec, {Name: "version", Kind: "readonly"}},
		results: []*adminpb.NodeResult{{Node: "a:1/svc", ResultJson: `{}`}},
		stream:  []*adminpb.InvokeStreamResponse{{Node: "a:1/svc", ResultJson: `{"message":"m"}`}},
	}
	out, _, err := run(t, f, "invoke", "svc", "logs", "--stream")
	if err != nil || !strings.Contains(out, "m") {
		t.Errorf("err = %v, out = %q: want the stream", err, out)
	}
	if _, _, err := run(t, f, "invoke", "svc", "version", "--stream"); err == nil || !strings.Contains(err.Error(), "does not stream") {
		t.Errorf("err = %v, want: does not stream", err)
	}
}
