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

// Like specs_notifications.go, this file imports no reva adapter, gateway or HTTP wiring.

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// TypeHandler processes a validated notification of one type. Its error
// never changes the response to a valid notification.
type TypeHandler interface {
	HandleNotification(ctx context.Context, n *Notification) error
}

// TypeHandlerFunc adapts a function to TypeHandler.
type TypeHandlerFunc func(ctx context.Context, n *Notification) error

// HandleNotification implements TypeHandler.
func (f TypeHandlerFunc) HandleNotification(ctx context.Context, n *Notification) error {
	return f(ctx, n)
}

// NotificationExtension is the contract of a notification type that is not
// built in, e.g. a scoped type defined by a later specification.
type NotificationExtension struct {
	Type     string
	Validate ValidateNotificationFunc
	Handler  TypeHandler
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

// NotificationRegistry holds the known notification contracts and their handlers.
type NotificationRegistry struct {
	mu    sync.RWMutex
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

// SetHandler installs the handler of a built-in or registered type.
func (r *NotificationRegistry) SetHandler(notificationType string, h TypeHandler) error {
	if h == nil {
		return fmt.Errorf("nil handler for notification type %s", notificationType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.types[notificationType]
	if !ok {
		return fmt.Errorf("notification type %s has no contract", notificationType)
	}
	e.handler = h
	return nil
}

// RegisterExtension adds the contract and handler of a type that is not
// built in. Built-in and request types cannot be redefined.
func (r *NotificationRegistry) RegisterExtension(ext NotificationExtension) error {
	if ext.Type == "" || ext.Validate == nil || ext.Handler == nil {
		return errors.New("incomplete notification extension")
	}
	if IsRequestNotification(ext.Type) {
		return fmt.Errorf("notification type %s is reserved", ext.Type)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.types[ext.Type]; ok {
		return fmt.Errorf("notification type %s already registered", ext.Type)
	}
	r.types[ext.Type] = &typeEntry{validate: ext.Validate, handler: ext.Handler}
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
		n.Class = NotificationClassRequest
		if err := validate(n); err != nil {
			return n, err
		}
		return n, &NotificationError{Code: NotificationErrNotImplemented}
	}
	r.mu.RLock()
	e, ok := r.types[n.Type]
	r.mu.RUnlock()
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
	r.mu.RLock()
	if e, ok := r.types[n.Type]; ok {
		h = e.handler
	}
	r.mu.RUnlock()
	if h == nil {
		return DispatchIgnored, nil
	}
	return DispatchHandled, h.HandleNotification(ctx, n)
}
