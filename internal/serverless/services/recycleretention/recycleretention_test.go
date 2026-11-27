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

package recycleretention

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// With a retention of 92 days, the cutoff is 27 August 2026.
var now = time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

// newTree creates <tmp>/project with the bins of the letters a and b. Every
// symlink in them points to a tree that looks like old day folders, so that the
// job removes it if it follows the symlink.
func newTree(t *testing.T) (tmp, bin string) {
	tmp = t.TempDir()
	outside := filepath.Join(tmp, "outside")
	bin = filepath.Join(tmp, "project", "a", ".recycle")
	for _, dir := range []string{
		filepath.Join(outside, "2020", "01", "01"),
		filepath.Join(bin, "sp", "2025", "12", "31", "dir"),
		filepath.Join(bin, "sp", "2026", "07", "01"),
		filepath.Join(bin, "sp", "2026", "08", "26", "dir"),
		filepath.Join(bin, "sp", "2026", "08", "27"),
		filepath.Join(bin, "sp", "2026", "11", "27"),
		filepath.Join(bin, "sp", "other", "2020", "01", "01"),
		filepath.Join(tmp, "project", "b"),
	} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
	}
	for _, file := range []string{
		filepath.Join(outside, "2020", "01", "01", "keep"),
		filepath.Join(bin, "sp", "2025", "12", "31", "dir", "file"),
		filepath.Join(bin, "sp", "2026", "08", "26", "dir", "file"),
		filepath.Join(bin, "sp", "2026", "08", "27", "file"),
	} {
		require.NoError(t, os.WriteFile(file, nil, 0o644))
	}
	for link, target := range map[string]string{
		filepath.Join(bin, "link"):                          outside,
		filepath.Join(bin, "sp", "2026", "06"):              filepath.Join(outside, "2020"),
		filepath.Join(bin, "sp", "2026", "08", "01"):        filepath.Join(outside, "2020", "01", "01"),
		filepath.Join(bin, "sp", "2026", "08", "26", "lnk"): outside,
		filepath.Join(tmp, "project", "b", ".recycle"):      tmp,
	} {
		require.NoError(t, os.Symlink(target, link))
	}
	return tmp, bin
}

func purge(tmp string, dryRun bool) error {
	s := &svc{conf: &config{
		Bins:          []string{filepath.Join(tmp, "project", "*", ".recycle")},
		RetentionDays: 92,
		DryRun:        dryRun,
	}}
	return s.purge(context.Background(), now)
}

func TestPurge(t *testing.T) {
	tmp, bin := newTree(t)

	err := purge(tmp, false)
	// the bin of b is a symlink
	require.ErrorContains(t, err, "is not a folder")

	for _, removed := range []string{
		filepath.Join(bin, "sp", "2025"),
		filepath.Join(bin, "sp", "2026", "07"),
		filepath.Join(bin, "sp", "2026", "08", "26"),
	} {
		assert.NoDirExists(t, removed)
	}
	for _, kept := range []string{
		filepath.Join(bin, "sp", "2026", "08", "27"),
		filepath.Join(bin, "sp", "2026", "11", "27"),
		filepath.Join(bin, "sp", "other", "2020", "01", "01"),
	} {
		assert.DirExists(t, kept)
	}
	for _, kept := range []string{
		filepath.Join(tmp, "outside", "2020", "01", "01", "keep"),
		filepath.Join(bin, "link"),
		filepath.Join(bin, "sp", "2026", "06"),
		filepath.Join(bin, "sp", "2026", "08", "01"),
	} {
		assert.FileExists(t, kept)
	}
}

func TestPurgeDryRun(t *testing.T) {
	tmp, bin := newTree(t)

	require.ErrorContains(t, purge(tmp, true), "is not a folder")

	assert.DirExists(t, filepath.Join(bin, "sp", "2025", "12", "31"))
	assert.DirExists(t, filepath.Join(bin, "sp", "2026", "07", "01"))
	assert.DirExists(t, filepath.Join(bin, "sp", "2026", "08", "26"))
}
