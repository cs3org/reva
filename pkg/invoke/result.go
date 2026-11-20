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

package invoke

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Error is an invocation error with a gRPC code. The control channel sends the
// code, so that a typed admin RPC can answer with it and not with Unknown.
type Error struct {
	Code    codes.Code
	Message string
}

func (e Error) Error() string { return e.Message }

// GRPCStatus lets status.Code and status.FromError read the code.
func (e Error) GRPCStatus() *status.Status { return status.New(e.Code, e.Message) }

// ProtoResult returns the protojson of m as a Result.
func ProtoResult(m proto.Message) (Result, error) {
	b, err := protojson.Marshal(m)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	return r, nil
}
