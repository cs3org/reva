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

package ocmd

import (
	"context"
	"errors"
)

// TypeHandler processes a validated notification and returns any effect error.
type TypeHandler interface {
	HandleNotification(ctx context.Context, n *Notification) error
}

// TypeHandlerFunc adapts a function to TypeHandler.
type TypeHandlerFunc func(ctx context.Context, n *Notification) error

// HandleNotification implements TypeHandler.
func (f TypeHandlerFunc) HandleNotification(ctx context.Context, n *Notification) error {
	return f(ctx, n)
}

// DispatchOutcome is the result of dispatching a parsed notification.
type DispatchOutcome string

const (
	// DispatchHandled means the type handler ran (its error, if any, is returned alongside).
	DispatchHandled DispatchOutcome = "handled"
	// DispatchIgnored means no handler is registered: accept and ignore.
	DispatchIgnored DispatchOutcome = "ignored"
	// DispatchNotImplemented means a request type that is never fulfilled.
	DispatchNotImplemented DispatchOutcome = "not_implemented"
)

type typeEntry struct {
	validate ValidateNotificationFunc
	handler  TypeHandler
}

// NotificationRegistry holds built-in contracts and their handlers.
// Complete registration before concurrent parsing or dispatch.
type NotificationRegistry struct {
	types map[string]*typeEntry
}

// NewNotificationRegistry returns a registry with the built-in contracts and
// no handlers; notifications of known types without a handler are ignored.
func NewNotificationRegistry() *NotificationRegistry {
	r := &NotificationRegistry{types: make(map[string]*typeEntry, len(builtinContracts))}
	for t, validate := range builtinContracts {
		r.types[t] = &typeEntry{validate: validate}
	}
	return r
}

// SetHandler installs or replaces a built-in handler during initialization.
func (r *NotificationRegistry) SetHandler(notificationType string, h TypeHandler) error {
	if h == nil {
		return errors.New("nil notification handler")
	}
	e, ok := r.types[notificationType]
	if !ok {
		return errors.New("notification type has no handler contract")
	}
	e.handler = h
	return nil
}

// Parse decodes the generic envelope and validates a known type. Valid
// requests return a not-implemented error; unknown types are not interpreted.
func (r *NotificationRegistry) Parse(body []byte) (*Notification, error) {
	n, err := decodeNotification(body)
	if err != nil {
		return &Notification{Class: NotificationClassMalformed}, err
	}
	if validate, ok := requestContracts[n.Type]; ok {
		if err := validate(n); err != nil {
			n.Class = NotificationClassMalformed
			return n, err
		}
		n.Class = NotificationClassRequest
		return n, &NotificationError{Code: NotificationErrNotImplemented}
	}
	e, ok := r.types[n.Type]
	if !ok {
		n.Class = NotificationClassUnknown
		return n, nil
	}
	// A known type with an invalid payload is never dispatched.
	if err := e.validate(n); err != nil {
		n.Class = NotificationClassMalformed
		return n, err
	}
	n.Class = NotificationClassSpec
	return n, nil
}

// Dispatch hands a parsed notification to its handler. Unknown types and
// known types without a handler are ignored.
func (r *NotificationRegistry) Dispatch(ctx context.Context, n *Notification) (DispatchOutcome, error) {
	switch n.Class {
	case NotificationClassRequest:
		return DispatchNotImplemented, nil
	case NotificationClassSpec:
	default:
		return DispatchIgnored, nil
	}
	var h TypeHandler
	if e, ok := r.types[n.Type]; ok {
		h = e.handler
	}
	if h == nil {
		return DispatchIgnored, nil
	}
	return DispatchHandled, h.HandleNotification(ctx, n)
}
