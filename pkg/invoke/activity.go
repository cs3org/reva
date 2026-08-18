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
	"time"
)

// ActivityInvocation is a built-in invocation every service instance exposes: it
// reports how many requests the instance's service is currently serving, how
// many it has served, and how long it has been idle — so an operator can tell
// when a (drained) instance has quiesced and is safe to restart.
const ActivityInvocation = "activity"

func init() {
	registerDefault(defaultInvocation{
		spec: InvocationSpec{
			Name:        ActivityInvocation,
			Description: "Report the instance's in-flight and recent request activity.",
			Kind:        KindReadonly,
		},
		fn: func(_ context.Context, inst instance, _ Args) (Result, error) {
			s := inst.activity.Snapshot()
			res := Result{"in_flight": s.InFlight, "total": s.Total}
			idleFields(res, s.LastRequest)
			if len(s.Methods) > 0 {
				methods := make([]map[string]any, 0, len(s.Methods))
				for name, m := range s.Methods {
					e := map[string]any{"method": name, "in_flight": m.InFlight, "total": m.Total}
					idleFields(e, m.LastRequest)
					methods = append(methods, e)
				}
				res["methods"] = methods
			}
			return res, nil
		},
	})
}

// idleFields writes idle_seconds and last_request onto a result from a request
// time (idle_seconds -1 and no last_request when nothing has been served).
func idleFields(m map[string]any, last time.Time) {
	if last.IsZero() {
		m["idle_seconds"] = -1.0
		m["last_request"] = ""
		return
	}
	m["idle_seconds"] = time.Since(last).Seconds()
	m["last_request"] = last.UTC().Format(time.RFC3339)
}
