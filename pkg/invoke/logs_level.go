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

	"github.com/cs3org/reva/v3/pkg/logger"
)

// LogLevelInvocation is a built-in invocation every service instance exposes: it
// reports, and optionally sets, the process's effective log level at runtime.
// The change is in-memory and process-wide; a restart reverts to the configured
// [log] level.
const LogLevelInvocation = "loglevel"

func init() {
	registerDefault(defaultInvocation{
		spec: InvocationSpec{
			Name:        LogLevelInvocation,
			Description: "Report or set the process's log level at runtime (reverts on restart).",
			Kind:        KindMutating,
			Args: []ArgSpec{
				{Name: "level", Description: "new level: trace|debug|info|warn|error (omit to report the current one)"},
			},
		},
		fn: func(_ context.Context, _ instance, a Args) (Result, error) {
			previous := logger.Level()
			level := previous
			if l := a.String("level"); l != "" {
				level = logger.SetLevel(l)
			}
			return Result{"previous": previous, "level": level}, nil
		},
	})
}
