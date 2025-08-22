//go:build ceph

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

package nceph

import (
	"testing"
)

// TestCephIntegrationFlagValidation skips when ceph build tag is present
// This ensures we don't get conflicts between the two versions of this test
func TestCephIntegrationFlagValidation(t *testing.T) {
	// When ceph build tag is present, the -ceph-integration flag is valid
	// The actual integration tests will validate proper configuration
	t.Skip("Ceph build tag present - integration flag validation not needed")
}
