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
