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
	"testing"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// TestRotationInvocation drains then re-enables an instance and checks that the
// drain flag and the reported state track it.
func TestRotationInvocation(t *testing.T) {
	id := "127.0.0.1:9810/svc-rot"
	RegisterInstance(id, "svc-rot", nil, nil, nil)
	t.Cleanup(func() { SetDrained(id, false) })

	if IsDrained(id) {
		t.Fatalf("instance should start in rotation")
	}

	res, err := Invoke(context.Background(), id, RotationInvocation, map[string]any{"state": "drain"})
	if err != nil {
		t.Fatalf("Invoke(rotation drain): %v", err)
	}
	if !IsDrained(id) {
		t.Fatalf("instance should be drained")
	}
	if res["previous"] != registry.StateReady || res["state"] != registry.StateDraining {
		t.Fatalf("unexpected drain result: %+v", res)
	}

	res, err = Invoke(context.Background(), id, RotationInvocation, map[string]any{"state": "ready"})
	if err != nil {
		t.Fatalf("Invoke(rotation ready): %v", err)
	}
	if IsDrained(id) {
		t.Fatalf("instance should be back in rotation")
	}
	if res["previous"] != registry.StateDraining || res["state"] != registry.StateReady {
		t.Fatalf("unexpected enable result: %+v", res)
	}

	if _, err := Invoke(context.Background(), id, RotationInvocation, map[string]any{"state": "sideways"}); err == nil {
		t.Fatalf("expected error for invalid state")
	}
}
