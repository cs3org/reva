// Copyright 2018-2024 CERN
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

package ocgraph

import (
	"bytes"
	"encoding/json"
	"testing"

	libregraph "github.com/owncloud/libre-graph-api-go"
)

func TestReceivedShareDriveItemAbsenceMatchesDriveItem(t *testing.T) {
	item := webdavOnlyDriveItem()
	multi := webdavOnlyDriveItem()
	multi.RemoteItem.Permissions = append(multi.RemoteItem.Permissions, secondGrant())

	tests := []struct {
		name string
		item *libregraph.DriveItem
		meta *receivedWebappMetadata
	}{
		{
			name: "nil metadata",
			item: item,
			meta: nil,
		},
		{
			name: "present false",
			item: item,
			meta: &receivedWebappMetadata{Present: false, AppName: "ignored"},
		},
		{
			name: "multiple permissions",
			item: multi,
			meta: nil,
		},
		{
			name: "omitted optional drive fields",
			item: sparseOptionalDriveItem(),
			meta: &receivedWebappMetadata{Present: false, AppName: "dropped"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := mustJSON(t, tt.item)
			got := mustJSON(t, newReceivedShareDriveItem(tt.item, tt.meta))
			if !bytes.Equal(got, want) {
				t.Fatalf("wrapper bytes differ\n got %s\nwant %s", got, want)
			}

			wantEnv := mustJSON(t, map[string]any{"value": []*libregraph.DriveItem{tt.item}})
			gotEnv := mustJSON(t, map[string]any{
				"value": []any{newReceivedShareDriveItem(tt.item, tt.meta)},
			})
			if !bytes.Equal(gotEnv, wantEnv) {
				t.Fatalf("envelope bytes differ\n got %s\nwant %s", gotEnv, wantEnv)
			}
		})
	}

	t.Run("nil item without metadata", func(t *testing.T) {
		want := mustJSON(t, (*libregraph.DriveItem)(nil))
		for _, meta := range []*receivedWebappMetadata{nil, {Present: false, AppName: "x"}} {
			got, err := json.Marshal(newReceivedShareDriveItem(nil, meta))
			if err != nil {
				t.Fatalf("marshal nil item: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("nil item bytes = %s, want %s", got, want)
			}
		}
	})

	t.Run("empty collection", func(t *testing.T) {
		var got bytes.Buffer
		if err := encodeSharedWithMe(&got, []any{}); err != nil {
			t.Fatal(err)
		}
		var env struct {
			Value []json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(got.Bytes(), &env); err != nil {
			t.Fatalf("decode empty envelope: %v; body %s", err, got.Bytes())
		}
		if len(env.Value) != 0 {
			t.Fatalf("empty value length = %d, want 0; body %s", len(env.Value), got.Bytes())
		}
	})
}

func TestReceivedShareDriveItemAddsWebappSiblingOnly(t *testing.T) {
	names := []string{
		`say "hi" / path\ok`,
		"<b>app</b>",
		"caf\u00e9 \u4e2d",
		"",
		"  text  ",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			item := webdavOnlyDriveItem()
			meta := &receivedWebappMetadata{Present: true, AppName: name}
			base := mustJSON(t, item)
			got := mustJSON(t, newReceivedShareDriveItem(item, meta))

			assertWebappSibling(t, base, got, name)
			var decoded struct {
				RemoteItem struct {
					Permissions []json.RawMessage `json:"permissions"`
				} `json:"remoteItem"`
			}
			if err := json.Unmarshal(got, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded.RemoteItem.Permissions) != 1 {
				t.Fatalf("permissions length = %d, want 1", len(decoded.RemoteItem.Permissions))
			}
		})
	}

	t.Run("additional grant preserved", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.RemoteItem.Permissions = append(item.RemoteItem.Permissions, secondGrant())
		base := mustJSON(t, item)
		got := mustJSON(t, newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}))
		assertWebappSibling(t, base, got, "text")

		var decoded struct {
			RemoteItem struct {
				Permissions []json.RawMessage `json:"permissions"`
			} `json:"remoteItem"`
		}
		if err := json.Unmarshal(got, &decoded); err != nil {
			t.Fatal(err)
		}
		if len(decoded.RemoteItem.Permissions) != 2 {
			t.Fatalf("permissions length = %d, want 2", len(decoded.RemoteItem.Permissions))
		}
	})
}

func TestReceivedShareDriveItemMetadataErrors(t *testing.T) {
	t.Run("nil item", func(t *testing.T) {
		got, err := newReceivedShareDriveItem(nil, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if got != nil {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("missing remoteItem", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.RemoteItem = nil
		got, err := newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if got != nil {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("base marshal failure", func(t *testing.T) {
		item := webdavOnlyDriveItem()
		item.Root = map[string]any{"bad": failJSON{}}
		got, err := newReceivedShareDriveItem(item, &receivedWebappMetadata{
			Present: true,
			AppName: "text",
		}).MarshalJSON()
		if err == nil {
			t.Fatal("expected error")
		}
		if len(got) != 0 {
			t.Fatalf("partial bytes %s", got)
		}
	})

	t.Run("extension encode failure", func(t *testing.T) {
		// remoteItem is an object, but a field value is not JSON, so the
		// extension cannot be written back.
		got, err := extendReceivedShareRemoteItem(
			[]byte(`{"id":"item-1","remoteItem":{"id":"remote-1","bad":oops}}`),
			"text",
		)
		if err == nil {
			t.Fatal("expected error")
		}
		if len(got) != 0 {
			t.Fatalf("partial bytes %s", got)
		}
	})

	cases := []struct {
		name string
		raw  string
	}{
		{"non-object remoteItem", `{"remoteItem":[]}`},
		{"null remoteItem", `{"remoteItem":null}`},
		{"string remoteItem", `{"remoteItem":"file"}`},
		{"malformed", `{`},
		{"root array", `[]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extendReceivedShareRemoteItem([]byte(tt.raw), "text")
			if err == nil {
				t.Fatal("expected error")
			}
			if got != nil {
				t.Fatalf("partial bytes %s", got)
			}
		})
	}
}
