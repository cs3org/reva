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

package backend

import (
	"errors"
	"fmt"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"response status", Check(&rpc.Status{Code: rpc.Code_CODE_NOT_FOUND}), true},
		{"wrapped response status", fmt.Errorf("get: %w", Check(&rpc.Status{Code: rpc.Code_CODE_NOT_FOUND})), true},
		{"other response status", Check(&rpc.Status{Code: rpc.Code_CODE_PERMISSION_DENIED, Message: "not found"}), false},
		{"provider error as grpc unknown", status.Error(codes.Unknown, "gateway: error calling GetShare: error getting share: share not found"), true},
		{"bad token", status.Error(codes.Unauthenticated, "auth: core access token not found"), false},
		{"scope failure", status.Error(codes.PermissionDenied, "access to resource not found in scope"), false},
		{"other grpc unknown", status.Error(codes.Unknown, "connection reset"), false},
		{"plain error", errors.New("not found"), false},
		{"nil", nil, false},
	} {
		if got := IsNotFound(tc.err); got != tc.want {
			t.Errorf("%s: IsNotFound(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}
