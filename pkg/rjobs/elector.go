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

package rjobs

import "context"

// Elector decides whether this replica should act as the scheduler for
// leader-scoped periodic jobs. It is an optimisation layer on top of the
// store: only the leader's scheduler enqueues due runs. Even if the elector
// is wrong for a moment (two leaders), the store's atomic next-fire advance
// prevents a duplicate enqueue, so the elector never affects correctness, only
// how much redundant scheduling work happens.
type Elector interface {
	// IsLeader reports whether this replica is currently the leader. It is
	// called frequently by the scheduler loop and must be cheap.
	IsLeader() bool
	// Close releases the elector's resources.
	Close(ctx context.Context) error
}

// AlwaysLeader is an Elector that always reports leadership. It is the right
// choice for a single-replica deployment and the default until a distributed
// elector (e.g. a NATS KV lock) is wired in.
type AlwaysLeader struct{}

// IsLeader always returns true.
func (AlwaysLeader) IsLeader() bool { return true }

// Close is a no-op.
func (AlwaysLeader) Close(context.Context) error { return nil }
