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
	"runtime"
	"strings"
)

// StackInvocation is a built-in invocation every service instance exposes: a
// dump of the process's goroutine stacks with basic runtime stats — the first
// tool to reach for when a process hangs.
const StackInvocation = "stack"

// maxStackDump caps the dump so the result stays well under the gRPC message
// limit; a hit is reported as truncated.
const maxStackDump = 2 << 20

func init() {
	registerDefault(defaultInvocation{
		spec: InvocationSpec{
			Name:        StackInvocation,
			Description: "Dump the process's goroutine stacks with basic runtime stats.",
			Kind:        KindReadonly,
			Args: []ArgSpec{
				{Name: "grep", Description: "keep only goroutines whose stack contains this substring"},
			},
		},
		fn: stackDump,
	})
}

func stackDump(_ context.Context, _ instance, a Args) (Result, error) {
	buf := make([]byte, 1<<20)
	truncated := false
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		if len(buf) >= maxStackDump {
			truncated = true
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	dump := string(buf)
	if g := a.String("grep"); g != "" {
		// Goroutine blocks are separated by blank lines.
		var kept []string
		for b := range strings.SplitSeq(dump, "\n\n") {
			if strings.Contains(b, g) {
				kept = append(kept, b)
			}
		}
		dump = strings.Join(kept, "\n\n")
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return Result{
		"goroutines":       runtime.NumGoroutine(),
		"gomaxprocs":       runtime.GOMAXPROCS(0),
		"heap_alloc_bytes": ms.HeapAlloc,
		"heap_sys_bytes":   ms.HeapSys,
		"num_gc":           ms.NumGC,
		"stacks":           dump,
		"truncated":        truncated,
	}, nil
}
