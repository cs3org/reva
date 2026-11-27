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

package invoke

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/cs3org/reva/v3/pkg/logger"
)

// TestLogCountsInvocation checks every instance reports its process's
// warn-and-above log counts, with what identifies the process.
func TestLogCountsInvocation(t *testing.T) {
	id := "127.0.0.1:9810/svc-lc"
	RegisterInstance(id, "svc-lc", nil, nil, nil)

	specs, _ := Invocations(id)
	if !hasSpec(specs, LogCountsInvocation) {
		t.Fatalf("expected %q in catalog: %+v", LogCountsInvocation, specs)
	}

	l := logger.New(logger.WithWriter(io.Discard, logger.JSONMode))
	res0, err := Invoke(context.Background(), id, LogCountsInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke(logcounts): %v", err)
	}
	l.Error().Msg("e")
	res1, err := Invoke(context.Background(), id, LogCountsInvocation, nil)
	if err != nil {
		t.Fatalf("Invoke(logcounts): %v", err)
	}

	c0, c1 := res0["counts"].(map[string]uint64), res1["counts"].(map[string]uint64)
	if c1["error"]-c0["error"] != 1 {
		t.Fatalf("error count went %d -> %d, want +1", c0["error"], c1["error"])
	}
	if res1["pid"] != os.Getpid() {
		t.Errorf("pid = %v, want %d", res1["pid"], os.Getpid())
	}
	if _, ok := res1["start"].(int64); !ok {
		t.Errorf("start = %#v, want unix seconds", res1["start"])
	}
	if _, ok := res1["config"].(string); !ok {
		t.Errorf("config = %#v, want the -c flag value (empty in tests)", res1["config"])
	}
}
