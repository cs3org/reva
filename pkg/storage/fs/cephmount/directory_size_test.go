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

package cephmount

import (
	"os"
	"path/filepath"
	"testing"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectorySize checks that a directory reports the size of everything below
// it, both when stat'ed directly and when seen as an entry of its parent listing.
func TestDirectorySize(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "directory-size-test")
	defer cleanup()

	// /folder holds an 11 byte file plus a subdirectory with a 9 byte one
	folder := filepath.Join(tempDir, "folder")
	require.NoError(t, os.MkdirAll(filepath.Join(folder, "sub"), 0777))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "a.txt"), []byte("hello world"), 0666))
	require.NoError(t, os.WriteFile(filepath.Join(folder, "sub", "b.txt"), []byte("nine byte"), 0666))
	require.NoError(t, os.Chmod(tempDir, 0777))

	const wantSize = uint64(20)

	config := map[string]any{
		"testing_allow_local_mode": true,
	}
	fs := CreateCephMountFSForTesting(t, ContextWithTestLogger(t), config, "/volumes/_nogroup/rasmus", tempDir)

	ctx := appctx.ContextSetUser(ContextWithTestLogger(t), GetCurrentTestUser(t))

	t.Run("GetMD_reports_recursive_size", func(t *testing.T) {
		ri, err := fs.GetMD(ctx, &provider.Reference{Path: "/folder"}, nil)
		require.NoError(t, err)
		assert.Equal(t, provider.ResourceType_RESOURCE_TYPE_CONTAINER, ri.Type)
		assert.Equal(t, wantSize, ri.Size, "folder should report the size of its whole subtree")
	})

	t.Run("ListFolder_reports_recursive_size_of_child", func(t *testing.T) {
		entries, err := fs.ListFolder(ctx, &provider.Reference{Path: "/"}, nil)
		require.NoError(t, err)

		var folderEntry *provider.ResourceInfo
		for _, entry := range entries {
			if entry.Path == "/folder" {
				folderEntry = entry
			}
		}
		require.NotNil(t, folderEntry, "listing of / should contain /folder")
		assert.Equal(t, wantSize, folderEntry.Size, "folder seen from its parent should report the same size")
	})
}
