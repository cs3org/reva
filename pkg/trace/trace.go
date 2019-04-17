// Copyright 2018-2019 CERN
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

package trace

import "context"

type key int

const traceKey key = iota

// ContextGetTrace returns the Trace if set in the given context.
func ContextGetTrace(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(traceKey).(string)
	return u, ok
}

// ContextMustGetTrace panics if Trace it not in context.
func ContextMustGetTrace(ctx context.Context) string {
	t, ok := ContextGetTrace(ctx)
	if !ok {
		panic("trace not found in context")
	}
	return t
}

// ContextSetTrace stores the trace in the context.
func ContextSetTrace(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, traceKey, trace)
}
