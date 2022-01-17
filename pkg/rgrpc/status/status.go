// Copyright 2018-2021 CERN
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

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/errtypes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewOK returns a Status with CODE_OK.
func NewOK(ctx context.Context) *rpc.Status {
	return &rpc.Status{
		Code:  rpc.Code_CODE_OK,
		Trace: getTrace(ctx),
	}
}

// NewNotFound returns a Status with CODE_NOT_FOUND.
func NewNotFound(ctx context.Context, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_NOT_FOUND,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInvalid returns a Status with CODE_INVALID_ARGUMENT.
func NewInvalid(ctx context.Context, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_INVALID_ARGUMENT,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInternal returns a Status with CODE_INTERNAL.
func NewInternal(ctx context.Context, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_INTERNAL,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnauthenticated returns a Status with CODE_UNAUTHENTICATED.
func NewUnauthenticated(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_UNAUTHENTICATED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewPermissionDenied returns a Status with PERMISSION_DENIED.
func NewPermissionDenied(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_PERMISSION_DENIED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInsufficientStorage returns a Status with INSUFFICIENT_STORAGE.
func NewInsufficientStorage(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_INSUFFICIENT_STORAGE,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewUnimplemented returns a Status with CODE_UNIMPLEMENTED.
func NewUnimplemented(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_UNIMPLEMENTED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewAlreadyExists returns a Status with CODE_ALREADY_EXISTS.
func NewAlreadyExists(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_ALREADY_EXISTS,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewInvalidArg returns a Status with CODE_INVALID_ARGUMENT.
func NewInvalidArg(ctx context.Context, msg string) *rpc.Status {
	return &rpc.Status{Code: rpc.Code_CODE_INVALID_ARGUMENT,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewConflict returns a Status with Code_CODE_ABORTED.
func NewConflict(ctx context.Context, err error, msg string) *rpc.Status {
	return &rpc.Status{
		Code:    rpc.Code_CODE_ABORTED,
		Message: msg,
		Trace:   getTrace(ctx),
	}
}

// NewStatusFromErrType returns a status that corresponds to the given errtype
func NewStatusFromErrType(ctx context.Context, msg string, err error) *rpc.Status {
	switch e := err.(type) {
	case nil:
		NewOK(ctx)
	case errtypes.IsNotFound:
		return NewNotFound(ctx, msg+": "+err.Error())
	case errtypes.IsInvalidCredentials:
		// TODO this maps badly
		return NewUnauthenticated(ctx, err, msg+": "+err.Error())
	case errtypes.PermissionDenied:
		return NewPermissionDenied(ctx, e, msg+": "+err.Error())
	case errtypes.IsNotSupported:
		return NewUnimplemented(ctx, err, msg+":"+err.Error())
	case errtypes.BadRequest:
		return NewInvalidArg(ctx, msg+":"+err.Error())
	}

	// map GRPC status codes coming from the auth middleware
	grpcErr := err
	for {
		st, ok := status.FromError(grpcErr)
		if ok {
			switch st.Code() {
			case codes.NotFound:
				return NewNotFound(ctx, msg+": "+err.Error())
			case codes.Unauthenticated:
				return NewUnauthenticated(ctx, err, msg+": "+err.Error())
			case codes.PermissionDenied:
				return NewPermissionDenied(ctx, err, msg+": "+err.Error())
			case codes.Unimplemented:
				return NewUnimplemented(ctx, err, msg+": "+err.Error())
			}
		}
		// the actual error can be wrapped multiple times
		grpcErr = errors.Unwrap(grpcErr)
		if grpcErr == nil {
			break
		}
	}

	return NewInternal(ctx, msg+":"+err.Error())
}

// NewErrorFromCode returns a standardized Error for a given RPC code.
func NewErrorFromCode(code rpc.Code, pkgname string) error {
	return errors.New(pkgname + ": grpc failed with code " + code.String())
}

// internal function to attach the trace to a context
func getTrace(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	return span.SpanContext().TraceID().String()
}
