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

package embedded

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

func parseGraph(t *testing.T, payload string) []crateEntity {
	t.Helper()
	var c crate
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	return c.Graph
}

func TestConfigApplyDefaults(t *testing.T) {
	t.Run("empty config gets defaults", func(t *testing.T) {
		c := Config{}
		c.ApplyDefaults()
		if c.Timeout != 3600 {
			t.Errorf("Timeout = %d, want 3600", c.Timeout)
		}
		if c.IdleTimeout != 120 {
			t.Errorf("IdleTimeout = %d, want 120", c.IdleTimeout)
		}
		if c.Retries != 3 {
			t.Errorf("Retries = %d, want 3", c.Retries)
		}
	})

	t.Run("explicit values preserved", func(t *testing.T) {
		c := Config{WebDAVURL: "https://dav.example.org/", Timeout: 10, IdleTimeout: 5, Retries: 1}
		c.ApplyDefaults()
		if c.Timeout != 10 || c.IdleTimeout != 5 || c.Retries != 1 {
			t.Errorf("explicit values were overwritten: %+v", c)
		}
		if c.WebDAVURL != "https://dav.example.org/" {
			t.Errorf("WebDAVURL = %q, want preserved", c.WebDAVURL)
		}
	})
}

func TestNewAppliesDefaults(t *testing.T) {
	tr, err := New(context.Background(), map[string]any{"webdav_url": "https://dav.example.org/"})
	if err != nil {
		t.Fatalf("New: unexpected error %v", err)
	}
	d := tr.(*driver)
	if d.c.Timeout != 3600 || d.c.IdleTimeout != 120 || d.c.Retries != 3 {
		t.Errorf("New did not apply defaults: %+v", d.c)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty", "", 0},
		{"seconds", "120", 120 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-5", 0},
		{"garbage", "abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.in); got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}

	t.Run("future http date", func(t *testing.T) {
		h := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
		got := parseRetryAfter(h)
		if got <= 0 || got > time.Hour {
			t.Errorf("parseRetryAfter(%q) = %v, want in (0, 1h]", h, got)
		}
	})

	t.Run("past http date", func(t *testing.T) {
		h := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
		if got := parseRetryAfter(h); got != 0 {
			t.Errorf("parseRetryAfter(past) = %v, want 0", got)
		}
	})
}

func TestZenodoFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://zenodo.org/records/1/files/data.csv/content", "data.csv"},
		{"https://example.org/path/file.bin", "file.bin"},
		{"https://example.org/file.txt/content", "file.txt"},
		{"plainname", "plainname"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := zenodoFilename(tt.in); got != tt.want {
				t.Errorf("zenodoFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCrateEntityURLString(t *testing.T) {
	tests := []struct {
		name string
		obj  string
		want string
	}{
		{"string url", `{"url":"https://x/y.txt"}`, "https://x/y.txt"},
		{"id object url", `{"url":{"@id":"https://x/z.bin"}}`, "https://x/z.bin"},
		{"absent url", `{"name":"foo"}`, ""},
		{"numeric url", `{"url":123}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.URLString(); got != tt.want {
				t.Errorf("URLString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCrateEntityHasType(t *testing.T) {
	tests := []struct {
		name    string
		obj     string
		want    string
		hasType bool
	}{
		{"single match", `{"@type":"File"}`, "File", true},
		{"single no match", `{"@type":"Dataset"}`, "File", false},
		{"array match", `{"@type":["Dataset","File"]}`, "File", true},
		{"array no match", `{"@type":["Dataset","Image"]}`, "File", false},
		{"absent", `{"name":"x"}`, "File", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.HasType(tt.want); got != tt.hasType {
				t.Errorf("HasType(%q) = %v, want %v", tt.want, got, tt.hasType)
			}
		})
	}
}

func TestCrateEntityIsTransferable(t *testing.T) {
	tests := []struct {
		name string
		obj  string
		want bool
	}{
		{"file with url", `{"@type":"File","url":"https://x/y"}`, true},
		{"workflow with url", `{"@type":"ComputationalWorkflow","url":"https://x/y"}`, true},
		{"file without url", `{"@type":"File"}`, false},
		{"dataset with url", `{"@type":"Dataset","url":"https://x/y"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e crateEntity
			if err := json.Unmarshal([]byte(tt.obj), &e); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := e.IsTransferable(); got != tt.want {
				t.Errorf("IsTransferable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCrateEntries(t *testing.T) {
	log := testLogger()

	tests := []struct {
		name    string
		payload string
		want    []transferEntry
	}{
		{
			name: "plain file with explicit name and size",
			payload: `{"@graph":[
				{"@id":"f1","@type":"File","url":"https://src.example.org/f1.txt","name":"f1.txt","contentSize":"1234","encodingFormat":"text/plain"}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/f1.txt", name: "f1.txt", sizeHint: 1234, encodingFormat: "text/plain"},
			},
		},
		{
			name: "plain id-object url, name derived from path, unknown size",
			payload: `{"@graph":[
				{"@id":"f2","@type":["File","Thing"],"url":{"@id":"https://src.example.org/path/f2.bin"}}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/path/f2.bin", name: "f2.bin", sizeHint: -1, encodingFormat: ""},
			},
		},
		{
			name: "zenodo dataset distributions",
			payload: `{"@graph":[
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/data.csv/content","encodingFormat":"text/csv"},
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/img.png/content"}
				]}
			]}`,
			want: []transferEntry{
				{srcURL: "https://zenodo.org/records/1/files/data.csv/content", name: "data.csv", sizeHint: -1, encodingFormat: "text/csv"},
				{srcURL: "https://zenodo.org/records/1/files/img.png/content", name: "img.png", sizeHint: -1, encodingFormat: ""},
			},
		},
		{
			name: "skips non-DataDownload and empty contentUrl distributions",
			payload: `{"@graph":[
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":""},
					{"@type":"WebPage","contentUrl":"https://zenodo.org/records/1"}
				]}
			]}`,
			want: nil,
		},
		{
			name: "skips non-transferable entity without url",
			payload: `{"@graph":[
				{"@id":"d","@type":"Dataset","name":"no url here"}
			]}`,
			want: nil,
		},
		{
			name: "mixed plain and zenodo, order preserved",
			payload: `{"@graph":[
				{"@id":"f1","@type":"File","url":"https://src.example.org/a.txt"},
				{"@id":"ds","@type":"Dataset","distribution":[
					{"@type":"DataDownload","contentUrl":"https://zenodo.org/records/1/files/b.csv/content"}
				]}
			]}`,
			want: []transferEntry{
				{srcURL: "https://src.example.org/a.txt", name: "a.txt", sizeHint: -1, encodingFormat: ""},
				{srcURL: "https://zenodo.org/records/1/files/b.csv/content", name: "b.csv", sizeHint: -1, encodingFormat: ""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crateEntries(log, parseGraph(t, tt.payload))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("crateEntries() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestProgressReader(t *testing.T) {
	const data = "hello world"
	before := time.Now().UnixNano()
	pr := newProgressReader(strings.NewReader(data))

	buf := make([]byte, 4)
	var total int64
	for {
		n, err := pr.Read(buf)
		total += int64(n)
		if err != nil {
			break
		}
	}

	if total != int64(len(data)) {
		t.Fatalf("read %d bytes, want %d", total, len(data))
	}
	if got := pr.total.Load(); got != int64(len(data)) {
		t.Errorf("pr.total = %d, want %d", got, len(data))
	}
	if pr.lastData.Load() < before {
		t.Errorf("lastData was not updated")
	}
}

func newTestTransferrer(t *testing.T) Transferrer {
	t.Helper()
	tr, err := New(context.Background(), map[string]any{"webdav_url": "https://dav.example.org/"})
	if err != nil {
		t.Fatalf("New: unexpected error %v", err)
	}
	return tr
}

func TestProcessMalformedPayload(t *testing.T) {
	tr := newTestTransferrer(t)
	if err := tr.Process(context.Background(), "{not valid json", "/dest"); err == nil {
		t.Error("Process with malformed payload: expected error, got nil")
	}
}

func TestProcessNoTransferableEntries(t *testing.T) {
	tr := newTestTransferrer(t)
	// A valid payload with nothing transferable must be a no-op: it returns nil
	// and does not require a token (no background transfer is started).
	payload := `{"@graph":[{"@id":"d","@type":"Dataset","name":"nothing to transfer"}]}`
	if err := tr.Process(context.Background(), payload, "/dest"); err != nil {
		t.Errorf("Process with no transferable entries: unexpected error %v", err)
	}
}
