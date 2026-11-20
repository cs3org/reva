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

package publicstorageprovider

import (
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/v3/pkg/spaces"
)

// a public link must never hand out the id of the resource behind it
func TestTranslateOpaqueFileID(t *testing.T) {
	realID := spaces.EncodeToStringifiedResourceID(&provider.ResourceId{
		StorageId: "storage-id",
		SpaceId:   "space-id",
		OpaqueId:  "inode",
	})
	publicID := spaces.EncodeToStringifiedResourceID(&provider.ResourceId{
		StorageId: "public-mount",
		SpaceId:   "space-id",
		OpaqueId:  "sometoken/inode",
	})

	// an empty wantID means the fileid should no longer be there
	tests := []struct {
		name    string
		mountID string
		opaque  *typesv1beta1.Opaque
		wantID  string
	}{
		{
			name:    "the fileid is rewritten into the public namespace",
			mountID: "public-mount",
			opaque:  opaqueWith(map[string]string{"fileid": realID, "etag": `"abc"`}),
			wantID:  publicID,
		},
		{
			name:    "an unparsable fileid is dropped",
			mountID: "public-mount",
			opaque:  opaqueWith(map[string]string{"fileid": "not-an-id", "etag": `"abc"`}),
		},
		{
			name:   "without a mount id the fileid is left alone",
			opaque: opaqueWith(map[string]string{"fileid": realID, "etag": `"abc"`}),
			wantID: realID,
		},
		{
			name:    "an opaque without a fileid is untouched",
			mountID: "public-mount",
			opaque:  opaqueWith(map[string]string{"etag": `"abc"`}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &service{mountID: tt.mountID}
			s.translateOpaqueFileID(tt.opaque, "sometoken")

			entry, ok := tt.opaque.Map["fileid"]
			switch {
			case tt.wantID == "":
				if ok {
					t.Fatalf("expected no fileid, got %q", entry.Value)
				}
			case !ok:
				t.Fatalf("expected fileid %q, got none", tt.wantID)
			case string(entry.Value) != tt.wantID:
				t.Fatalf("got fileid %q, want %q", entry.Value, tt.wantID)
			}

			// the etag rides next to the fileid and is never rewritten
			if e, ok := tt.opaque.Map["etag"]; !ok || string(e.Value) != `"abc"` {
				t.Fatalf("etag changed: %v", tt.opaque.Map["etag"])
			}
		})
	}
}

func TestTranslateOpaqueFileIDOnNilOpaque(t *testing.T) {
	s := &service{mountID: "public-mount"}
	s.translateOpaqueFileID(nil, "sometoken")
}

func opaqueWith(entries map[string]string) *typesv1beta1.Opaque {
	o := &typesv1beta1.Opaque{Map: map[string]*typesv1beta1.OpaqueEntry{}}
	for k, v := range entries {
		o.Map[k] = &typesv1beta1.OpaqueEntry{Decoder: "plain", Value: []byte(v)}
	}
	return o
}
