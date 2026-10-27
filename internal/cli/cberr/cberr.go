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

// Package cberr holds the errors of the CLI: a short sentence and a kind that
// maps to a stable exit code.
//
// Scripts branch on the exit code. Exit 3 means a new login can help, exit 4
// means it cannot. So code must set the kind where it makes the error, and
// must not guess it later from the message.
package cberr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind classifies an error.
type Kind int

const (
	// KindOther is an unclassified failure.
	KindOther Kind = iota
	// KindUsage is a malformed command line.
	KindUsage
	// KindAuth means the credentials are missing, expired, or rejected.
	KindAuth
	// KindPermission means the identity is known but not allowed.
	KindPermission
	// KindNotFound means the resource does not exist.
	KindNotFound
	// KindConflict means the target is in a state that does not allow the
	// request: a lock, a changed etag, a full quota, a non-empty directory.
	KindConflict
)

// Exit codes. Scripts depend on them.
const (
	ExitOK         = 0
	ExitFailure    = 1
	ExitUsage      = 2
	ExitAuth       = 3
	ExitPermission = 4
	ExitNotFound   = 5
	ExitConflict   = 6
)

func (k Kind) String() string {
	switch k {
	case KindUsage:
		return "usage"
	case KindAuth:
		return "authentication"
	case KindPermission:
		return "permission"
	case KindNotFound:
		return "not found"
	case KindConflict:
		return "conflict"
	default:
		return "error"
	}
}

// Error is an error with a kind. Op and Path build the message, for example
// "cannot stat /eos/user/g/x: not found".
type Error struct {
	Kind Kind
	// Op is a verb: "stat", "upload".
	Op   string
	Path string
	// Status is the HTTP status of the response, or 0.
	Status int
	// Msg is the sentence. When empty, Kind gives a default.
	Msg string
	// Err is the cause, kept for --debug and errors.Is/As.
	Err error
}

func (e *Error) Error() string {
	msg := e.Msg
	if msg == "" {
		msg = e.Kind.String()
	}
	switch {
	case e.Op != "" && e.Path != "":
		return fmt.Sprintf("cannot %s %s: %s", e.Op, e.Path, msg)
	case e.Op != "":
		return fmt.Sprintf("cannot %s: %s", e.Op, msg)
	default:
		return msg
	}
}

func (e *Error) Unwrap() error { return e.Err }

// Is makes errors.Is(err, ErrAuth) a test on the kind.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return t.Kind == e.Kind && t.Op == "" && t.Path == "" && t.Msg == ""
}

// Sentinels for errors.Is.
var (
	ErrAuth     = &Error{Kind: KindAuth}
	ErrNotFound = &Error{Kind: KindNotFound}
)

// New returns an Error.
func New(kind Kind, op, path, msg string) *Error {
	return &Error{Kind: kind, Op: op, Path: path, Msg: msg}
}

// Wrap returns an Error with err as the cause.
func Wrap(kind Kind, op, path string, err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, Path: path, Msg: err.Error(), Err: err}
}

// Usagef returns a usage Error.
func Usagef(format string, args ...any) *Error {
	return &Error{Kind: KindUsage, Msg: fmt.Sprintf(format, args...)}
}

// Authf returns an authentication Error.
func Authf(format string, args ...any) *Error {
	return &Error{Kind: KindAuth, Msg: fmt.Sprintf(format, args...)}
}

// FromStatus returns an Error for an HTTP status. In WebDAV, 405, 412, 423 and
// 507 all mean that the target does not allow the request, so they are
// conflicts.
func FromStatus(status int, op, path, msg string) *Error {
	var kind Kind
	switch status {
	case http.StatusUnauthorized:
		kind = KindAuth
	case http.StatusForbidden:
		kind = KindPermission
	case http.StatusNotFound, http.StatusGone:
		kind = KindNotFound
	case http.StatusMethodNotAllowed,
		http.StatusConflict,
		http.StatusPreconditionFailed,
		http.StatusLocked,
		http.StatusInsufficientStorage:
		kind = KindConflict
	default:
		kind = KindOther
	}
	if msg == "" {
		msg = defaultMessage(status)
	}
	return &Error{Kind: kind, Op: op, Path: path, Status: status, Msg: msg}
}

func defaultMessage(status int) string {
	switch status {
	case http.StatusUnauthorized:
		// Not "log in again". A missing credential fails earlier. A 401 means
		// the server refused the credential that was sent. A new login does not
		// help: the token can be valid and the server still not map it to an
		// account.
		return "the server rejected the credentials: 'status' shows which one was used; --debug shows the detail"
	case http.StatusForbidden:
		return "permission denied"
	case http.StatusNotFound, http.StatusGone:
		return "no such file or directory"
	case http.StatusConflict:
		return "conflicting state: a parent directory may be missing"
	case http.StatusPreconditionFailed:
		return "the resource changed on the server since it was read"
	case http.StatusLocked:
		return "the resource is locked by another client"
	case http.StatusInsufficientStorage:
		return "quota exceeded"
	case http.StatusMethodNotAllowed:
		return "the server rejected this operation on this resource"
	default:
		return fmt.Sprintf("server returned %d %s", status, http.StatusText(status))
	}
}

// KindOf returns the kind of err, or KindOther.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return KindOther
}

// ExitCode returns the exit code for err.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	switch KindOf(err) {
	case KindUsage:
		return ExitUsage
	case KindAuth:
		return ExitAuth
	case KindPermission:
		return ExitPermission
	case KindNotFound:
		return ExitNotFound
	case KindConflict:
		return ExitConflict
	default:
		return ExitFailure
	}
}
