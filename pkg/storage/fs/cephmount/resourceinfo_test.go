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

package cephmount

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceInfoNameAndEtag covers the fields clients need to render a shared
// resource: without them a received share shows up unnamed in the web UI.
func TestResourceInfoNameAndEtag(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "resourceinfo")
	t.Cleanup(cleanup)

	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "folder"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "folder", "report.pdf"), []byte("data"), 0644))

	ctx := ContextWithTestLogger(t)
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{"testing_allow_local_mode": true}, "/volumes/_nogroup/test", tempDir)

	tests := []struct {
		path         string
		expectedName string
	}{
		{path: "/folder/report.pdf", expectedName: "report.pdf"},
		{path: "/folder", expectedName: "folder"},
		{path: "/", expectedName: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			ri, err := fs.GetMD(ctx, &provider.Reference{Path: tt.path}, nil)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedName, ri.Name)
			assert.NotEmpty(t, ri.Etag)
			assert.Equal(t, `"`, string(ri.Etag[0]), "etag should be quoted")
		})
	}
}

func TestEtagChangesWhenFileIsModified(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "etag-changes")
	t.Cleanup(cleanup)

	filePath := filepath.Join(tempDir, "file.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("before"), 0644))

	ctx := ContextWithTestLogger(t)
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{"testing_allow_local_mode": true}, "/volumes/_nogroup/test", tempDir)
	ref := &provider.Reference{Path: "/file.txt"}

	before, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)

	// Two stats of an unmodified file must agree, otherwise clients re-download.
	again, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	assert.Equal(t, before.Etag, again.Etag, "etag must be stable while the file is untouched")

	require.NoError(t, os.Chtimes(filePath, time.Now().Add(time.Second), time.Now().Add(time.Second)))

	after, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	assert.NotEqual(t, before.Etag, after.Etag, "etag must change when the file is modified")
}
