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

// MLSIngress receives the MLS notification types. Their bytes bypass
// legacy normalization, because they feed cryptographic processing.
type MLSIngress interface {
	HandleMLSNotification(ctx context.Context, n *Notification) error
}

// ErrMLSIngressUnavailable reports an MLS notification received while no
// MLS ingress is installed.
var ErrMLSIngressUnavailable = errors.New("mls notification ingress unavailable")

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
	// DispatchMLSUnavailable means an MLS type arrived without an MLS ingress.
	DispatchMLSUnavailable DispatchOutcome = "mls_unavailable"
)

type typeEntry struct {
	contractType string
	validate     ValidateNotificationFunc
	handler      TypeHandler
}

// NotificationRegistry holds the known notification contracts and their handlers.
type NotificationRegistry struct {
	mu    sync.RWMutex
	types map[string]*typeEntry
	mls   MLSIngress
}

// NewNotificationRegistry returns a registry with the built-in contracts and
// no handlers; notifications of known types without a handler are ignored.
func NewNotificationRegistry() *NotificationRegistry {
	r := &NotificationRegistry{types: make(map[string]*typeEntry, len(builtinContracts))}
	for t, c := range builtinContracts {
		r.types[t] = &typeEntry{contractType: c.contractType, validate: c.validate}
	}
	return r
}

// SetHandler installs the handler of a built-in or registered type.
// RESHARE_UNDO has its own handler slot.
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
// built in. Built-in, request and MLS types cannot be redefined.
func (r *NotificationRegistry) RegisterExtension(ext NotificationExtension) error {
	if ext.Type == "" || ext.Validate == nil || ext.Handler == nil {
		return errors.New("incomplete notification extension")
	}
	if IsRequestNotification(ext.Type) || IsMLSNotification(ext.Type) {
		return fmt.Errorf("notification type %s is reserved", ext.Type)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.types[ext.Type]; ok {
		return fmt.Errorf("notification type %s already registered", ext.Type)
	}
	r.types[ext.Type] = &typeEntry{contractType: ext.Type, validate: ext.Validate, handler: ext.Handler}
	return nil
}

// SetMLSIngress installs the MLS ingress. A nil ingress keeps MLS
// notifications reported as unavailable.
func (r *NotificationRegistry) SetMLSIngress(m MLSIngress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mls = m
}

// Parse decodes, classifies, normalizes and validates a notification body.
// Unknown and MLS types are returned without normalization.
func (r *NotificationRegistry) Parse(body []byte) (*Notification, error) {
	n, err := decodeNotification(body)
	if err != nil {
		return &Notification{Class: NotificationClassMalformed}, err
	}
	switch {
	case IsRequestNotification(n.Type):
		n.Class = NotificationClassRequest
		return n, &NotificationError{Code: NotificationErrNotImplemented}
	case IsMLSNotification(n.Type):
		n.Class = NotificationClassMLS
		return n, nil
	}
	r.mu.RLock()
	e, ok := r.types[n.Type]
	r.mu.RUnlock()
	if !ok {
		n.Class = NotificationClassUnknown
		return n, nil
	}
	if err := finishKnown(n, e.contractType, e.validate); err != nil {
		return n, err
	}
	return n, nil
}

// Dispatch hands a parsed notification to its handler. Unknown types and
// known types without a handler are ignored.
func (r *NotificationRegistry) Dispatch(ctx context.Context, n *Notification) (DispatchOutcome, error) {
	switch n.Class {
	case NotificationClassRequest:
		return DispatchNotImplemented, nil
	case NotificationClassMLS:
		r.mu.RLock()
		m := r.mls
		r.mu.RUnlock()
		if m == nil {
			return DispatchMLSUnavailable, ErrMLSIngressUnavailable
		}
		return DispatchHandled, m.HandleMLSNotification(ctx, n)
	case NotificationClassSpec, NotificationClassLegacy:
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
