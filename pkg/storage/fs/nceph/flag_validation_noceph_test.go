//go:build !ceph

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

// TestCephIntegrationFlagValidation ensures that the -ceph-integration flag
// is only used with the 'ceph' build tag. This test runs when the ceph build tag is NOT present.
func TestCephIntegrationFlagValidation(t *testing.T) {
	if *cephIntegration {
		t.Fatal("The -ceph-integration flag was used but no Ceph integration tests are available. " +
			"You must use the 'ceph' build tag: go test -tags ceph -ceph-integration -v")
	}
	// If the flag is not set, this test passes silently
}
