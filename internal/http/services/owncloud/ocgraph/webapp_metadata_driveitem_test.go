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

func TestSharedWithMeContractFixtures(t *testing.T) {
	fixtures := loadReceivedWebappFixturesStrict(t)
	item := goldenReceivedShareDriveItem(t, fixtures.WebDAVOnly)

	tests := []struct {
		name        string
		meta        *receivedWebappMetadata
		wantFixture []byte
		wantWebapp  bool
		appName     string
	}{
		{
			name:        "nil metadata",
			meta:        nil,
			wantFixture: fixtures.WebDAVOnly,
		},
		{
			name:        "present false",
			meta:        &receivedWebappMetadata{Present: false, AppName: "ignored"},
			wantFixture: fixtures.WebDAVOnly,
		},
		{
			name:        "present true CodiMD",
			meta:        &receivedWebappMetadata{Present: true, AppName: "CodiMD"},
			wantFixture: fixtures.Positive,
			wantWebapp:  true,
			appName:     "CodiMD",
		},
		{
			name:        "present true empty name",
			meta:        &receivedWebappMetadata{Present: true, AppName: ""},
			wantFixture: fixtures.EmptyName,
			wantWebapp:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.meta == nil || !tt.meta.Present {
				want := mustJSON(t, item)
				got := mustJSON(t, newReceivedShareDriveItem(item, tt.meta))
				if !bytes.Equal(got, want) {
					t.Fatalf("wrapper bytes differ\n got %s\nwant %s", got, want)
				}
			}

			var buf bytes.Buffer
			if err := encodeSharedWithMe(&buf, []any{newReceivedShareDriveItem(item, tt.meta)}); err != nil {
				t.Fatal(err)
			}
			assertSharedWithMeEnvelopeEqual(t, buf.Bytes(), tt.wantFixture)

			values := decodeSharedWithMeValues(t, buf.Bytes())
			if len(values) != 1 {
				t.Fatalf("value length = %d, want 1", len(values))
			}
			root := decodeObject(t, values[0])
			if _, ok := root["folder"]; !ok {
				t.Fatal("folder missing on drive item")
			}

			if tt.wantWebapp {
				assertWebappSibling(t, mustJSON(t, item), values[0], tt.appName)
				return
			}
			assertPermissionCount(t, values[0], 1)
			remote := decodeObject(t, root["remoteItem"])
			if _, ok := remote[receivedWebappJSONKey]; ok {
				t.Fatal("unexpected remoteItem webapp sibling")
			}
		})
	}
}

func TestReceivedWebappFixtureManifest(t *testing.T) {
	fixtures := loadReceivedWebappFixturesStrict(t)
	if fixtures.Expectations.Revision != receivedWebappExpectationsRevision {
		t.Fatalf("revision = %q, want %q", fixtures.Expectations.Revision, receivedWebappExpectationsRevision)
	}
	if len(fixtures.Expectations.Cases) == 0 {
		t.Fatal("resource-expectations cases empty")
	}

	knownResponses := map[string][]byte{
		"positive.sharedWithMe.json":    fixtures.Positive,
		"webdav-only.sharedWithMe.json": fixtures.WebDAVOnly,
		"empty-name.sharedWithMe.json":  fixtures.EmptyName,
	}
	seen := make(map[string]struct{}, len(fixtures.Expectations.Cases))
	for _, c := range fixtures.Expectations.Cases {
		if c.Response == "" {
			t.Fatal("case response filename empty")
		}
		raw, ok := knownResponses[c.Response]
		if !ok {
			t.Fatalf("unknown response filename %q", c.Response)
		}
		wantHash, ok := fixtures.Sums[c.Response]
		if !ok {
			t.Fatalf("SHA256SUMS missing response %q", c.Response)
		}
		if got := sha256Hex(raw); got != wantHash {
			t.Fatalf("%s digest = %s, want %s", c.Response, got, wantHash)
		}
		seen[c.Response] = struct{}{}
	}
	for name := range knownResponses {
		if _, ok := seen[name]; !ok {
			t.Fatalf("resource-expectations missing case for %q", name)
		}
	}
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
