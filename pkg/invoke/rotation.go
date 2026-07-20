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
	"fmt"

	"github.com/cs3org/reva/v3/pkg/registry"
)

// RotationInvocation is a built-in invocation that takes an instance out of
// rotation (state=drain) or returns it (state=ready) at runtime. The change is
// in-memory and per node: a restart reverts it. The runtime's heartbeat
// advertises the resulting registry state, and the service selector excludes
// draining nodes, so a drained instance receives no new traffic while staying
// alive and control-reachable.
const RotationInvocation = "rotation"

func init() {
	registerDefault(defaultInvocation{
		spec: InvocationSpec{
			Name:        RotationInvocation,
			Description: "Take the instance out of rotation (state=drain) or return it (state=ready).",
			Kind:        KindDangerous,
			Args: []ArgSpec{
				{Name: "state", Description: "drain|ready", Required: true},
			},
		},
		fn: func(_ context.Context, inst instance, a Args) (Result, error) {
			previous := rotationState(inst.id)
			switch a.String("state") {
			case "drain", "draining", "out":
				SetDrained(inst.id, true)
			case "ready", "in", "enable":
				SetDrained(inst.id, false)
			default:
				return nil, fmt.Errorf("state must be drain or ready")
			}
			return Result{"node": inst.id, "previous": previous, "state": rotationState(inst.id)}, nil
		},
	})
}

// rotationState maps the drain flag to the registry state name it advertises.
func rotationState(id string) string {
	if IsDrained(id) {
		return registry.StateDraining
	}
	return registry.StateReady
}
