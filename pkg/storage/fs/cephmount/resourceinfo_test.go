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
	"syscall"
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

// Without a ceph mount there is no rctime, and the etag still has to be valid,
// stable, and move when a direct entry changes.
func TestDirectoryEtagWithoutRecursiveCtime(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "etag-directory")
	t.Cleanup(cleanup)

	dir := filepath.Join(tempDir, "folder")
	require.NoError(t, os.MkdirAll(dir, 0755))

	ctx := ContextWithTestLogger(t)
	fs := CreateCephMountFSForTesting(t, ctx, map[string]any{"testing_allow_local_mode": true}, "/volumes/_nogroup/test", tempDir)
	ref := &provider.Reference{Path: "/folder"}

	before, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	assert.Equal(t, `"`, string(before.Etag[0]), "etag should be quoted")

	again, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	assert.Equal(t, before.Etag, again.Etag, "etag must be stable while the directory is untouched")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "child.txt"), []byte("x"), 0644))
	require.NoError(t, os.Chtimes(dir, time.Now().Add(time.Second), time.Now().Add(time.Second)))

	after, err := fs.GetMD(ctx, ref, nil)
	require.NoError(t, err)
	assert.NotEqual(t, before.Etag, after.Etag, "etag must change when a direct child changes")
}

// A change below a directory reaches its etag, which is what makes a sync
// client descend into it.
func TestCalcEtagIncludesRecursiveCtime(t *testing.T) {
	tempDir, cleanup := GetTestDir(t, "etag-rctime")
	t.Cleanup(cleanup)

	info, err := os.Stat(tempDir)
	require.NoError(t, err)
	stat := info.Sys().(*syscall.Stat_t)

	without := calcEtag(info, stat, "")
	with := calcEtag(info, stat, "1690000000.000000000")
	later := calcEtag(info, stat, "1690000001.000000000")

	assert.Equal(t, `"`, string(with[0]), "etag should be quoted")
	assert.NotEqual(t, without, with, "a subtree change must change the etag")
	assert.NotEqual(t, with, later, "a later subtree change must change the etag again")
	assert.Equal(t, without, calcEtag(info, stat, ""), "the fallback must be stable")
}

func TestRctimeSeconds(t *testing.T) {
	tests := []struct {
		rctime  string
		seconds uint64
		ok      bool
	}{
		{rctime: "1690000000.000000000", seconds: 1690000000, ok: true},
		{rctime: "1690000000.09123456789", seconds: 1690000000, ok: true},
		{rctime: "1690000000", seconds: 1690000000, ok: true},
		{rctime: "", ok: false},
		{rctime: "not-a-time", ok: false},
	}

	for _, tt := range tests {
		seconds, ok := rctimeSeconds(tt.rctime)
		assert.Equal(t, tt.ok, ok, tt.rctime)
		assert.Equal(t, tt.seconds, seconds, tt.rctime)
	}
}
