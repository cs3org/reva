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
	"errors"

	rpcpb "github.com/cs3org/go-cs3apis/cs3/rpc"
	"github.com/cs3org/reva/pkg/appctx"
	"go.opencensus.io/trace"
)

// NewOK returns a Status with CODE_OK.
func NewOK(ctx context.Context) *rpcpb.Status {
	return &rpcpb.Status{
		Code:  rpcpb.Code_CODE_OK,
		Trace: getTrace(ctx),
	}
}

// NewNotFound returns a Status with CODE_NOT_FOUND and logs the msg.
func NewNotFound(ctx context.Context, err error, msg string) *rpcpb.Status {
	if err != nil {
		appctx.GetLogger(ctx).Err(err).Msg(msg)
	}
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_NOT_FOUND,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInvalid returns a Status with CODE_INVALID and logs the msg.
func NewInvalid(ctx context.Context, err error, msg string) *rpcpb.Status {
	if err != nil {
		appctx.GetLogger(ctx).Err(err).Msg(msg)
	}
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_INVALID,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInternal returns a Status with CODE_INTERNAL and logs the msg.
func NewInternal(ctx context.Context, err error, msg string) *rpcpb.Status {
	if err != nil {
		appctx.GetLogger(ctx).Err(err).Msg(msg)
	}
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_INTERNAL,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnauthenticated returns a Status with CODE_UNAUTHENTICATED and logs the msg.
func NewUnauthenticated(ctx context.Context, err error, msg string) *rpcpb.Status {
	if err != nil {
		appctx.GetLogger(ctx).Err(err).Msg(msg)
	}
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_UNAUTHENTICATED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnimplemented returns a Status with CODE_UNIMPLEMENTED and logs the msg.
func NewUnimplemented(ctx context.Context, err error, msg string) *rpcpb.Status {
	if err != nil {
		appctx.GetLogger(ctx).Err(err).Msg(msg)
	}
	return &rpcpb.Status{
		Code:    rpcpb.Code_CODE_UNIMPLEMENTED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewErrorFromCode returns a standardized Error for a given RPC code.
func NewErrorFromCode(code rpcpb.Code, pkgname string) error {
	return errors.New(pkgname + ": RPC failed with code " + code.String())
}

// internal function to attach the trace to a context
func getTrace(ctx context.Context) string {
	span := trace.FromContext(ctx)
	return span.SpanContext().TraceID.String()
}
