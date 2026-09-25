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

	"github.com/rs/zerolog"
)

func decodeSharedWithMeItem(t *testing.T, raw []byte) []byte {
	t.Helper()
	values := decodeSharedWithMeValues(t, raw)
	if len(values) != 1 {
		t.Fatalf("value length = %d, want 1", len(values))
	}
	return values[0]
}

func decodeSharedWithMeValues(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	envelope := decodeObject(t, raw)
	var values []json.RawMessage
	if err := json.Unmarshal(envelope["value"], &values); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	return values
}

func assertPermissionCount(t *testing.T, item []byte, want int) {
	t.Helper()
	root := decodeObject(t, item)
	if _, ok := root[receivedWebappJSONKey]; ok {
		t.Fatalf("%s placed on drive item", receivedWebappJSONKey)
	}
	remote := decodeObject(t, root["remoteItem"])
	if _, ok := remote[receivedWebappJSONKey]; ok {
		t.Fatalf("%s present on remoteItem", receivedWebappJSONKey)
	}
	var perms []json.RawMessage
	if err := json.Unmarshal(remote["permissions"], &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) != want {
		t.Fatalf("permissions length = %d, want %d", len(perms), want)
	}
}

func assertWebappSibling(t *testing.T, base, got []byte, appName string) {
	t.Helper()

	baseRoot := decodeObject(t, base)
	gotRoot := decodeObject(t, got)
	if _, ok := gotRoot[receivedWebappJSONKey]; ok {
		t.Fatalf("%s placed on drive item", receivedWebappJSONKey)
	}
	if len(gotRoot) != len(baseRoot) {
		t.Fatalf("root keys = %d, want %d", len(gotRoot), len(baseRoot))
	}
	for key, raw := range baseRoot {
		if key == "remoteItem" {
			continue
		}
		if !bytes.Equal(gotRoot[key], raw) {
			t.Fatalf("root field %s changed\n got %s\nwant %s", key, gotRoot[key], raw)
		}
	}

	baseRemote := decodeObject(t, baseRoot["remoteItem"])
	gotRemote := decodeObject(t, gotRoot["remoteItem"])
	rawWebapp, ok := gotRemote[receivedWebappJSONKey]
	if !ok {
		t.Fatal("missing remoteItem webapp")
	}
	if len(gotRemote) != len(baseRemote)+1 {
		t.Fatalf("remoteItem keys = %d, want %d", len(gotRemote), len(baseRemote)+1)
	}
	for key, raw := range baseRemote {
		if !bytes.Equal(gotRemote[key], raw) {
			t.Fatalf("remoteItem field %s changed\n got %s\nwant %s", key, gotRemote[key], raw)
		}
	}

	var perms []map[string]json.RawMessage
	if err := json.Unmarshal(gotRemote["permissions"], &perms); err != nil {
		t.Fatal(err)
	}
	if len(perms) < 1 {
		t.Fatal("permissions empty")
	}
	if _, ok := perms[0]["grantedToV2"]; !ok {
		t.Fatal("grantedToV2 missing on permissions[0]")
	}
	if _, ok := perms[0]["invitation"]; !ok {
		t.Fatal("invitation missing on permissions[0]")
	}
	if !bytes.Equal(perms[0]["grantedToV2"], decodePermField(t, baseRemote["permissions"], 0, "grantedToV2")) {
		t.Fatal("grantedToV2 changed")
	}
	if !bytes.Equal(perms[0]["invitation"], decodePermField(t, baseRemote["permissions"], 0, "invitation")) {
		t.Fatal("invitation changed")
	}

	webapp := decodeObject(t, rawWebapp)
	if len(webapp) != 1 {
		t.Fatalf("webapp keys = %d, want 1", len(webapp))
	}
	var decodedName string
	if err := json.Unmarshal(webapp["appName"], &decodedName); err != nil {
		t.Fatal(err)
	}
	if decodedName != appName {
		t.Fatalf("appName = %q, want %q", decodedName, appName)
	}
	for _, forbidden := range []string{
		"secret",
		"token",
		"http://",
		"https://",
		"@libre.graph.permissions.actions",
	} {
		if bytes.Contains(rawWebapp, []byte(forbidden)) {
			t.Fatalf("webapp contains %s: %s", forbidden, rawWebapp)
		}
	}
}

func decodeObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decode object: %v; raw %s", err, raw)
	}
	return obj
}

func decodePermField(t *testing.T, raw []byte, index int, field string) json.RawMessage {
	t.Helper()
	var perms []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &perms); err != nil {
		t.Fatal(err)
	}
	return perms[index][field]
}

func captureLogger() (*bytes.Buffer, zerolog.Logger) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Timestamp().Logger()
	return &buf, logger
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
