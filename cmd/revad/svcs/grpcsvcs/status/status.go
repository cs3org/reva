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

// Package status contains helpers functions
// to create grpc Status with contextual information,
// like traces.
package status

import (
	"context"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"go.opencensus.io/trace"
)

// NewOK returns a Status with CODE_OK.
func NewOK(ctx context.Context) *rpcpb.Status {
	return &rpcpb.Status{
		Code:  rpcpb.Code_CODE_OK,
		Trace: getTrace(ctx),
	}
}

// NewNotFound returns a Status with CODE_NOT_FOUND.
func NewNotFound(ctx context.Context, msg string) *rpcpb.Status {
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_NOT_FOUND,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInternal returns a Status with CODE_INTERNAL.
func NewInternal(ctx context.Context, msg string) *rpcpb.Status {
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_INTERNAL,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnauthenticated returns a Status with CODE_UNAUTHENTICATED.
func NewUnauthenticated(ctx context.Context, msg string) *rpcpb.Status {
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_UNAUTHENTICATED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnimplemented returns a Status with CODE_UNIMPLEMENTED.
func NewUnimplemented(ctx context.Context, msg string) *rpcpb.Status {
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_UNIMPLEMENTED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

func getTrace(ctx context.Context) string {
	span := trace.FromContext(ctx)
	return span.SpanContext().TraceID.String()
}
