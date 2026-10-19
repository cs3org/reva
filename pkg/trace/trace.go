// Copyright 2018-2024 CERN
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

import (
	"context"

	"github.com/gofrs/uuid"
)

type key struct{}

type jobKey struct{}

func Get(ctx context.Context) (t string) {
	t, _ = ctx.Value(key{}).(string)
	return
}

func Generate() string {
	return uuid.Must(uuid.NewV4()).String()
}

// ContextSetTrace stores the trace in the context.
func Set(ctx context.Context, trace string) context.Context {
	return context.WithValue(ctx, key{}, trace)
}

// GetJob returns the name of the background job that caused the request.
func GetJob(ctx context.Context) (job string) {
	job, _ = ctx.Value(jobKey{}).(string)
	return
}

// SetJob stores the name of the background job in the context. It goes to
// other services with the trace id, so their logs can be filtered by job.
func SetJob(ctx context.Context, job string) context.Context {
	return context.WithValue(ctx, jobKey{}, job)
}
