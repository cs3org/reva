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

package status

import (
	"context"
	"errors"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"google.golang.org/grpc/codes"
)

func TestNewStatusFromErrType(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want rpc.Code
	}{
		{name: "nil", err: nil, want: rpc.Code_CODE_OK},
		{name: "not found", err: errtypes.NotFound("x"), want: rpc.Code_CODE_NOT_FOUND},
		{name: "permission denied", err: errtypes.PermissionDenied("x"), want: rpc.Code_CODE_PERMISSION_DENIED},
		{name: "invalid credentials", err: errtypes.InvalidCredentials("x"), want: rpc.Code_CODE_UNAUTHENTICATED},
		{name: "user required", err: errtypes.UserRequired("x"), want: rpc.Code_CODE_UNAUTHENTICATED},
		{name: "not supported", err: errtypes.NotSupported("x"), want: rpc.Code_CODE_UNIMPLEMENTED},
		{name: "bad request", err: errtypes.BadRequest("x"), want: rpc.Code_CODE_INVALID_ARGUMENT},
		{name: "already exists", err: errtypes.AlreadyExists("x"), want: rpc.Code_CODE_ALREADY_EXISTS},
		{name: "conflict", err: errtypes.Conflict("x"), want: rpc.Code_CODE_ABORTED},
		{name: "insufficient storage", err: errtypes.InsufficientStorage("x"), want: rpc.Code_CODE_INSUFFICIENT_STORAGE},
		{name: "unknown", err: errors.New("boom"), want: rpc.Code_CODE_INTERNAL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewStatusFromErrType(context.Background(), "op", tt.err)
			if got.Code != tt.want {
				t.Fatalf("code = %v, want %v", got.Code, tt.want)
			}
		})
	}
}

func TestNewRedactedStatusFromErrType(t *testing.T) {
	got := NewRedactedStatusFromErrType(context.Background(), "op", errtypes.NotFound("secret detail"))
	if got.Code != rpc.Code_CODE_NOT_FOUND {
		t.Fatalf("code = %v, want %v", got.Code, rpc.Code_CODE_NOT_FOUND)
	}
	if got.Message != "op" {
		t.Fatalf("message = %q, want %q", got.Message, "op")
	}

	got = NewRedactedStatusFromErrType(context.Background(), "op", errors.New("db unreachable"))
	if got.Code != rpc.Code_CODE_INTERNAL || got.Message != "op" {
		t.Fatalf("got %v %q, want INTERNAL %q", got.Code, got.Message, "op")
	}
}

func TestCodeFromGRPC(t *testing.T) {
	for in, want := range map[codes.Code]rpc.Code{
		codes.OK:                 rpc.Code_CODE_OK,
		codes.Canceled:           rpc.Code_CODE_CANCELLED,
		codes.Unknown:            rpc.Code_CODE_UNKNOWN,
		codes.InvalidArgument:    rpc.Code_CODE_INVALID_ARGUMENT,
		codes.DeadlineExceeded:   rpc.Code_CODE_DEADLINE_EXCEEDED,
		codes.NotFound:           rpc.Code_CODE_NOT_FOUND,
		codes.AlreadyExists:      rpc.Code_CODE_ALREADY_EXISTS,
		codes.PermissionDenied:   rpc.Code_CODE_PERMISSION_DENIED,
		codes.ResourceExhausted:  rpc.Code_CODE_RESOURCE_EXHAUSTED,
		codes.FailedPrecondition: rpc.Code_CODE_FAILED_PRECONDITION,
		codes.Aborted:            rpc.Code_CODE_ABORTED,
		codes.OutOfRange:         rpc.Code_CODE_OUT_OF_RANGE,
		codes.Unimplemented:      rpc.Code_CODE_UNIMPLEMENTED,
		codes.Internal:           rpc.Code_CODE_INTERNAL,
		codes.Unavailable:        rpc.Code_CODE_UNAVAILABLE,
		codes.DataLoss:           rpc.Code_CODE_DATA_LOSS,
		codes.Unauthenticated:    rpc.Code_CODE_UNAUTHENTICATED,
		codes.Code(99):           rpc.Code_CODE_UNKNOWN,
	} {
		if got := CodeFromGRPC(in); got != want {
			t.Errorf("CodeFromGRPC(%v) = %v, want %v", in, got, want)
		}
	}
}
