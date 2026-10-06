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

import (
	"context"
	"time"
)

// pruneBatch is how many runs PruneRuns deletes per statement.
const pruneBatch = 10000

// PruneRuns deletes the terminal runs that finished before before, in batches,
// and returns how many it deleted. It is a no-op when the status store cannot
// prune.
func (r *Runner) PruneRuns(ctx context.Context, before time.Time) (int, error) {
	p, ok := r.status.(RunPruner)
	if !ok {
		return 0, nil
	}
	total := 0
	for {
		n, err := p.Prune(ctx, before, pruneBatch)
		total += n
		if err != nil || n < pruneBatch {
			return total, err
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}
