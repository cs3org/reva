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

package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The error codes a tool answers with.
const (
	codeNotFound           = "not_found"
	codePermissionDenied   = "permission_denied"
	codeNotPermittedByAP   = "not_permitted_by_app_password"
	codeOutsideRoots       = "outside_app_password_roots"
	codeTooLarge           = "too_large"
	codeUnsupportedType    = "unsupported_type"
	codePreconditionFailed = "precondition_failed"
	codeRateLimited        = "rate_limited"
	codeInvalidArgument    = "invalid_argument"
	codeInternal           = "internal"

	codeOK = "ok"
)

// toolError is the result of a failed tool call. Its text is a JSON object, so
// that a client can tell the code apart from the message.
type toolError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
	WebURL  string `json:"web_url,omitempty"`
}

func newError(code, msg string) *toolError {
	return &toolError{Code: code, Message: msg}
}

func (e *toolError) Error() string {
	b, _ := json.Marshal(e)
	return string(b)
}

// errorCode is the code a tool call ended with, for the audit log.
func errorCode(err error) string {
	if err == nil {
		return codeOK
	}
	if te, ok := errors.AsType[*toolError](err); ok {
		return te.Code
	}
	return codeInternal
}

// asToolError turns a failed gateway call into a tool error. A gRPC permission
// denied comes from the auth interceptor of a service the call went through.
func asToolError(err error, what string) *toolError {
	if te, ok := errors.AsType[*toolError](err); ok {
		return te
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.PermissionDenied, codes.Unauthenticated:
			return newError(codePermissionDenied, "permission denied on "+what)
		case codes.NotFound:
			return newError(codeNotFound, what+" not found")
		}
	}
	return newError(codeInternal, fmt.Sprintf("error on %s: %v", what, err))
}

// checkStatus turns a CS3 status other than OK into a tool error.
func checkStatus(st *rpc.Status, what string) error {
	var code string
	switch st.GetCode() {
	case rpc.Code_CODE_OK:
		return nil
	case rpc.Code_CODE_NOT_FOUND:
		return newError(codeNotFound, what+" not found")
	case rpc.Code_CODE_PERMISSION_DENIED, rpc.Code_CODE_UNAUTHENTICATED:
		return newError(codePermissionDenied, "permission denied on "+what)
	case rpc.Code_CODE_ALREADY_EXISTS:
		return newError(codePreconditionFailed, what+" already exists")
	case rpc.Code_CODE_FAILED_PRECONDITION, rpc.Code_CODE_ABORTED, rpc.Code_CODE_LOCKED:
		code = codePreconditionFailed
	case rpc.Code_CODE_INVALID_ARGUMENT, rpc.Code_CODE_OUT_OF_RANGE:
		code = codeInvalidArgument
	case rpc.Code_CODE_INSUFFICIENT_STORAGE:
		return newError(codeTooLarge, "not enough quota for "+what)
	case rpc.Code_CODE_RESOURCE_EXHAUSTED:
		code = codeRateLimited
	default:
		code = codeInternal
	}
	msg := what + ": " + st.GetCode().String()
	if st.GetMessage() != "" {
		msg = what + ": " + st.GetMessage()
	}
	return newError(code, msg)
}

// checkHTTP turns the status of a data transfer into a tool error.
func checkHTTP(code int, what string) error {
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusNotFound:
		return newError(codeNotFound, what+" not found")
	case code == http.StatusForbidden || code == http.StatusUnauthorized:
		return newError(codePermissionDenied, "permission denied on "+what)
	case code == http.StatusPreconditionFailed || code == http.StatusLocked || code == http.StatusConflict:
		return newError(codePreconditionFailed, fmt.Sprintf("%s: %s", what, http.StatusText(code)))
	case code == http.StatusRequestEntityTooLarge || code == http.StatusInsufficientStorage:
		return newError(codeTooLarge, fmt.Sprintf("%s: %s", what, http.StatusText(code)))
	}
	return newError(codeInternal, fmt.Sprintf("%s: %s", what, http.StatusText(code)))
}
