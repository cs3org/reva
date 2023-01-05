// Copyright 2018-2023 CERN
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

package mock

import (
	"context"
	"io/fs"
	"path/filepath"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	typesv1beta1 "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/storage/utils/walker"
)

type mockWalker struct{}

// NewWalker creates a mock walker that implements the Walk interface
// supposed to be used for testing.
func NewWalker() walker.Walker {
	return &mockWalker{}
}

// converts a FileInfo to a reva ResourceInfo.
func convertFileInfoToResourceInfo(path string, f fs.FileInfo) *provider.ResourceInfo {
	if f == nil {
		return nil
	}
	// resource type conversion
	t := provider.ResourceType_RESOURCE_TYPE_FILE
	if f.IsDir() {
		t = provider.ResourceType_RESOURCE_TYPE_CONTAINER
	}
	return &provider.ResourceInfo{
		Type: t,
		Path: path,
		Size: uint64(f.Size()),
		Mtime: &typesv1beta1.Timestamp{
			Seconds: uint64(f.ModTime().Second()),
		},
	}
}

func mockWalkFunc(fn walker.WalkFunc) filepath.WalkFunc {
	return func(path string, info fs.FileInfo, err error) error {
		return fn(path, convertFileInfoToResourceInfo(path, info), err)
	}
}

// Walk walks into the local file system using the built-in filepath.Walk go function.
func (m *mockWalker) Walk(_ context.Context, root string, fn walker.WalkFunc) error {
	return filepath.Walk(root, mockWalkFunc(fn))
}
