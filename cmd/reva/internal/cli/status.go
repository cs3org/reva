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

package cli

import (
	"errors"
	"fmt"
	"strings"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StatusError is a CS3 response status that is not OK. Commands return it
// so that a caller can read the code, not only the text.
type StatusError struct {
	Code  rpc.Code
	Msg   string
	Trace string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("code=%s msg=%q support_trace=%q", e.Code, e.Msg, e.Trace)
}

// Check returns nil for an OK status, else a *StatusError.
func Check(s *rpc.Status) error {
	if s.GetCode() == rpc.Code_CODE_OK {
		return nil
	}
	return &StatusError{Code: s.GetCode(), Msg: s.GetMessage(), Trace: s.GetTrace()}
}

// IsNotFound reports whether err is a not-found answer of a CS3 call. Some
// providers still return their not-found as a Go error, which arrives as
// gRPC Unknown with the text "not found", so that form counts too. Other
// gRPC errors (a bad token, a scope failure) never count.
func IsNotFound(err error) bool {
	if se, ok := errors.AsType[*StatusError](err); ok {
		return se.Code == rpc.Code_CODE_NOT_FOUND
	}
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.Unknown && strings.Contains(strings.ToLower(s.Message()), "not found")
}
