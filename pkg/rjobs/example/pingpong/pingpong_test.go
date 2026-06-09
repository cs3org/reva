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

package pingpong

import (
	"context"
	"testing"

	"github.com/cs3org/reva/v3/pkg/rjobs"
)

func TestRun(t *testing.T) {
	j, err := New(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := j.Run(context.Background(), rjobs.Params{"ping": "hello"}); err != nil {
		t.Errorf("Run with a ping should succeed, got %v", err)
	}

	if err := j.Run(context.Background(), rjobs.Params{}); err == nil {
		t.Error("Run without a ping should fail")
	}
}

func TestEnqueueWithoutRunner(t *testing.T) {
	rjobs.SetDefault(nil)
	if _, err := Enqueue(context.Background(), "hello"); err == nil {
		t.Error("Enqueue should fail when the jobs service is not enabled")
	}
}
