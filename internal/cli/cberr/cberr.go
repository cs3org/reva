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

// Package cberr models errors in the terms the CLI's callers care about: a
// short human sentence, and a category that maps to a stable exit code.
//
// The category is not decoration. Scripts branch on it: exit 3 means the
// credentials need refreshing and the command is worth retrying after kinit,
// exit 4 means the user simply does not have access and retrying will never
// help. Keeping that distinction accurate is the reason errors are classified
// at the point they are produced rather than pattern-matched later.
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
	// KindConflict covers locks, etag mismatches, quota, and non-empty
	// directories: states where the request was understood but the target is
	// not in a shape that allows it.
	KindConflict
)

// Exit codes, documented in the README and depended on by scripts.
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

// Error is a classified failure. Op and Path are optional context used to build
// the message, so that the user sees "cannot stat /eos/user/g/x: not found"
// rather than a bare status code.
type Error struct {
	Kind Kind
	// Op is what was being attempted, phrased as a verb: "stat", "upload".
	Op string
	// Path is the resource involved, if any.
	Path string
	// Status is the HTTP status that produced this error, or 0.
	Status int
	// Msg is the human sentence. When empty, Kind supplies a default.
	Msg string
	// Err is the underlying cause, kept for --debug and errors.Is/As.
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

// Is supports errors.Is(err, &Error{Kind: KindAuth}) as a category test.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return t.Kind == e.Kind && t.Op == "" && t.Path == "" && t.Msg == ""
}

// Sentinels for errors.Is category tests.
var (
	ErrAuth       = &Error{Kind: KindAuth}
	ErrPermission = &Error{Kind: KindPermission}
	ErrNotFound   = &Error{Kind: KindNotFound}
	ErrConflict   = &Error{Kind: KindConflict}
	ErrUsage      = &Error{Kind: KindUsage}
)

// New builds a classified error.
func New(kind Kind, op, path, msg string) *Error {
	return &Error{Kind: kind, Op: op, Path: path, Msg: msg}
}

// Wrap builds a classified error around an existing one.
func Wrap(kind Kind, op, path string, err error) *Error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Op: op, Path: path, Msg: err.Error(), Err: err}
}

// Usagef builds a usage error.
func Usagef(format string, args ...any) *Error {
	return &Error{Kind: KindUsage, Msg: fmt.Sprintf(format, args...)}
}

// Authf builds an authentication error.
func Authf(format string, args ...any) *Error {
	return &Error{Kind: KindAuth, Msg: fmt.Sprintf(format, args...)}
}

// FromStatus classifies an HTTP status code.
//
// 405, 412, 423 and 507 land in KindConflict because in WebDAV they all mean
// the same thing to a user: the request was understood, but the target is not
// in a state that allows it: a non-empty collection, a changed etag, a lock,
// or a full quota.
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
		// Deliberately not "log in again". Having no credential at all fails
		// earlier, with its own message; by the time a request comes back 401
		// the CLI did send one and the server refused it. Telling the user to
		// authenticate again sends them round a loop that cannot help: and a
		// token the server rejects can be perfectly valid, with the deployment
		// unable to map it to an account.
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

// KindOf extracts the category of an error, defaulting to KindOther.
func KindOf(err error) Kind {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Kind
	}
	return KindOther
}

// ExitCode maps an error to the process exit code.
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
