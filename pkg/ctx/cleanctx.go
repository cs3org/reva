// Copyright 2018-2023 CERN
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

package ctx

import (
	"context"
  "time"
)

type cleanCtx struct {
  ctx context.Context
}

// ContextGetClean returns a new, clean context derived by the given one
func ContextGetClean(ctx context.Context) context.Context {
	return cleanCtx {
    ctx: ctx,
  }
}


func (c cleanCtx) Deadline () (time.Time, bool) {
  return c.ctx.Deadline()
}

func (c cleanCtx) Done () <-chan struct{} {
  return c.ctx.Done()
}

func (c cleanCtx) Err () error {
  return c.ctx.Err()
}

func (c cleanCtx) Value (key any) any {
  return nil
}
