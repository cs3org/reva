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

package net

import (
	"fmt"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

type rpcStatusGetter interface {
	GetStatus() *rpc.Status
}

// CheckRPCInvocation checks if an RPC invocation has succeeded.
// For this, the error from the original call is first checked; after that, the actual RPC response status is checked.
func CheckRPCInvocation(operation string, res rpcStatusGetter, callErr error) error {
	if callErr != nil {
		return fmt.Errorf("%s: %v", operation, callErr)
	}

	return CheckRPCStatus(operation, res)
}

// CheckRPCStatus checks the returned status of an RPC call.
func CheckRPCStatus(operation string, res rpcStatusGetter) error {
	status := res.GetStatus()
	if status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("%s: %q (code=%+v, trace=%q)", operation, status.Message, status.Code, status.Trace)
	}

	return nil
}
