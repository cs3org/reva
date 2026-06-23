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

package nats

import (
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
)

func TestSanitizeKey(t *testing.T) {
	got := keyFor("storageprovider", "host:127.0.0.1:9142#42/storageprovider")
	for i, r := range got {
		if i == len("storageprovider") {
			if r != '.' {
				t.Fatalf("expected '.' separator at %d, got %q", i, r)
			}
			continue
		}
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			t.Fatalf("illegal char %q in key %q", r, got)
		}
	}
}

// TestOfflineQueuesWriteThrough verifies that, with NATS unreachable, the
// driver does not fail and queues the write to flush on connect.
func TestOfflineQueuesWriteThrough(t *testing.T) {
	drv, err := New(map[string]any{"address": "nats://127.0.0.1:14222"}) // nothing listening
	if err != nil {
		t.Fatalf("New should not fail: %v", err)
	}
	d := drv.(*driver)
	defer d.Close()

	if err := d.Add("gateway", registry.NewNode("n1", "10.0.0.1:19000", map[string]string{
		registry.MetaState: registry.StateReady,
	})); err != nil {
		t.Fatalf("Add returned error while offline: %v", err)
	}

	d.mu.Lock()
	_, queued := d.pending[keyFor("gateway", "n1")]
	d.mu.Unlock()
	if !queued {
		t.Fatal("expected the write to be queued while disconnected")
	}
}
