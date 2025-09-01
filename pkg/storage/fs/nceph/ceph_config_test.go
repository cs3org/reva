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

//go:build !ceph

package nceph

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCephConfigHelper(t *testing.T) {
	// Test configuration behavior without fstab entry set
	t.Run("no_fstab_entry", func(t *testing.T) {
		// Ensure no fstab entry is set
		originalFstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY")
		os.Unsetenv("NCEPH_FSTAB_ENTRY")
		defer func() {
			if originalFstabEntry != "" {
				os.Setenv("NCEPH_FSTAB_ENTRY", originalFstabEntry)
			}
		}()

		config := GetCephConfig()
		
		// Should return empty config when no fstab entry is provided
		assert.Empty(t, config, "Config should be empty when no NCEPH_FSTAB_ENTRY is set")
		assert.NotContains(t, config, "fstab_entry", "Should not contain fstab_entry when none is set")
	})

	// Test configuration with fstab entry set
	t.Run("with_fstab_entry", func(t *testing.T) {
		// Set a test fstab entry
		testFstabEntry := "cephfs.cephfs /mnt/cephfs ceph defaults,name=admin,secretfile=/etc/ceph/ceph.client.admin.keyring,conf=/etc/ceph/ceph.conf 0 2"
		
		originalFstabEntry := os.Getenv("NCEPH_FSTAB_ENTRY")
		os.Setenv("NCEPH_FSTAB_ENTRY", testFstabEntry)
		defer func() {
			if originalFstabEntry == "" {
				os.Unsetenv("NCEPH_FSTAB_ENTRY")
			} else {
				os.Setenv("NCEPH_FSTAB_ENTRY", originalFstabEntry)
			}
		}()

		config := GetCephConfig()
		
		// Should have fstab_entry field when environment variable is set
		assert.Contains(t, config, "fstab_entry")
		assert.Equal(t, testFstabEntry, config["fstab_entry"])
		t.Logf("Fstab entry: %s", config["fstab_entry"])
	})
}
