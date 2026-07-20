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

package admin

import (
	"context"
	"testing"
)

// TestFanOutInvokeReportsPerNodeErrors checks that unresolved/offline nodes are
// reported per-node (in order) rather than failing the whole fan-out.
func TestFanOutInvokeReportsPerNodeErrors(t *testing.T) {
	eps := []endpoint{
		{node: "n1", err: "node advertises no control endpoint"},
		{node: "n2", err: "offline"},
	}
	res := fanOutInvoke(context.Background(), eps, "op", nil)
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Node != "n1" || res[0].Error == "" {
		t.Errorf("n1 result wrong: %+v", res[0])
	}
	if res[1].Node != "n2" || res[1].Error == "" {
		t.Errorf("n2 result wrong: %+v", res[1])
	}
}
