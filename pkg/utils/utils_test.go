// Copyright 2018-2022 CERN
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

package utils

import (
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

var skipTests = []struct {
	name string
	url  string
	base []string
	out  bool
}{
	{"valid subpath", "/a/b/c/d", []string{"/a/b/"}, true},
	{"invalid subpath", "/a/b/c", []string{"/a/b/c/d"}, false},
	{"equal values", "/a/b/c", []string{"/a/b/c"}, true},
}

func TestSkip(t *testing.T) {
	for _, tt := range skipTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r := Skip(tt.url, tt.base)
			if r != tt.out {
				t.Errorf("expected %v, want %v", r, tt.out)
			}
		})
	}
}
func TestIsRelativeReference(t *testing.T) {
	tests := []struct {
		ref      *provider.Reference
		expected bool
	}{
		{
			&provider.Reference{},
			false,
		},
		{
			&provider.Reference{
				Path: ".",
			},
			false,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storageId",
					OpaqueId:  "opaqueId",
				},
				Path: "/folder",
			},
			false,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storageId",
					OpaqueId:  "opaqueId",
				},
				Path: "./folder",
			},
			true,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{},
				Path:       "./folder",
			},
			true,
		},
	}

	for _, tt := range tests {
		result := IsRelativeReference(tt.ref)
		if result != tt.expected {
			t.Errorf("IsRelativeReference: ref %v expected %t got %t", tt.ref, tt.expected, result)
		}
	}
}
func TestIsAbsolutReference(t *testing.T) {
	tests := []struct {
		ref      *provider.Reference
		expected bool
	}{
		{
			&provider.Reference{},
			false,
		},
		{
			&provider.Reference{
				Path: ".",
			},
			false,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storageId",
					OpaqueId:  "opaqueId",
				},
				Path: "/folder",
			},
			false,
		},
		{
			&provider.Reference{
				Path: "/folder",
			},
			true,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{},
			},
			true,
		},
		{
			&provider.Reference{
				ResourceId: &provider.ResourceId{
					StorageId: "storageId",
					OpaqueId:  "opaqueId",
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		result := IsAbsoluteReference(tt.ref)
		if result != tt.expected {
			t.Errorf("IsAbsolutReference: ref %v expected %t got %t", tt.ref, tt.expected, result)
		}
	}
}

func TestMakeRelativePath(t *testing.T) {
	tests := []struct {
		path    string
		relPath string
	}{
		{"", "."},
		{"/", "."},
		{"..", "."},
		{"/folder", "./folder"},
		{"/folder/../folder2", "./folder2"},
		{"folder", "./folder"},
	}
	for _, tt := range tests {
		rel := MakeRelativePath(tt.path)
		if rel != tt.relPath {
			t.Errorf("expected %s, got %s", tt.relPath, rel)
		}
	}
}

func TestParseStorageSpaceReference(t *testing.T) {
	tests := []struct {
		sRef      string
		storageID string
		nodeID    string
		relPath   string
	}{
		{
			"1234!abcd/f1/f2",
			"1234",
			"abcd",
			"./f1/f2",
		},
		{
			"1234!abcd",
			"1234",
			"abcd",
			".",
		},
	}
	for _, tt := range tests {
		ref, _ := ParseStorageSpaceReference(tt.sRef)

		if ref.ResourceId.StorageId != tt.storageID {
			t.Errorf("Expected storageId %s got %s", tt.storageID, ref.ResourceId.StorageId)
		}
		if ref.ResourceId.OpaqueId != tt.nodeID {
			t.Errorf("Expected OpaqueId %s got %s", tt.nodeID, ref.ResourceId.OpaqueId)
		}
		if ref.Path != tt.relPath {
			t.Errorf("Expected path %s got %s", tt.relPath, ref.Path)
		}
	}
}

func TestHasPermissions(t *testing.T) {
	tests := []struct {
		name     string
		target   *provider.ResourcePermissions
		toCheck  *provider.ResourcePermissions
		expected bool
	}{
		{
			name:     "both empty",
			target:   &provider.ResourcePermissions{},
			toCheck:  &provider.ResourcePermissions{},
			expected: true,
		},
		{
			name:   "empty target",
			target: &provider.ResourcePermissions{},
			toCheck: &provider.ResourcePermissions{
				AddGrant: true,
			},
			expected: false,
		},
		{
			name: "empty to_check",
			target: &provider.ResourcePermissions{
				AddGrant: true,
			},
			toCheck:  &provider.ResourcePermissions{},
			expected: true,
		},
		{
			name: "to_check is a subset",
			target: &provider.ResourcePermissions{
				AddGrant:        true,
				CreateContainer: true,
				Delete:          true,
				GetPath:         true,
			},
			toCheck: &provider.ResourcePermissions{
				CreateContainer: true,
				GetPath:         true,
			},
			expected: true,
		},
		{
			name: "to_check contains permissions to in target",
			target: &provider.ResourcePermissions{
				AddGrant:        true,
				CreateContainer: true,
				Delete:          true,
				GetPath:         true,
			},
			toCheck: &provider.ResourcePermissions{
				CreateContainer: true,
				GetPath:         true,
				Move:            true,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if res := HasPermissions(tt.target, tt.toCheck); res != tt.expected {
				t.Fatalf("got unexpected result: target=%+v to_check=%+v res=%+v expected=%+v", tt.target, tt.toCheck, res, tt.expected)
			}
		})
	}
}
