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

package reqid

import "context"
import "github.com/gofrs/uuid"

type key int

const reqIDKey key = iota

// ReqIDHeaderName is the header to use when storing the
// request ID into an HTTP or GRPC header.
const ReqIDHeaderName = "x-request-id"

// ContextGetReqID returns the reqID if set in the given context.
func ContextGetReqID(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(reqIDKey).(string)
	return u, ok
}

// ContextMustGetReqID panics if reqID it not in context.
func ContextMustGetReqID(ctx context.Context) string {
	t, ok := ContextGetReqID(ctx)
	if !ok {
		panic("reqID not found in context")
	}
	return t
}

// ContextSetReqID stores the reqID in the context.
func ContextSetReqID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, reqIDKey, reqID)
}

// MintReqID creates a new request id.
func MintReqID() string {
	return uuid.Must(uuid.NewV4()).String()
}
