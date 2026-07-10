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

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/cs3org/reva/v3/pkg/notifications/model"
)

// Handler executes one delivery action for a notification envelope.
type Handler interface {
	Name() string
	Send(ctx context.Context, envelope model.Envelope) error
}

// Dispatcher resolves and invokes the handlers requested by an envelope.
type Dispatcher struct {
	handlers map[string]Handler
}

// NewDispatcher creates a handler dispatcher.
func NewDispatcher(handlers ...Handler) *Dispatcher {
	d := &Dispatcher{
		handlers: make(map[string]Handler, len(handlers)),
	}

	for _, h := range handlers {
		if h == nil {
			continue
		}
		d.handlers[h.Name()] = h
	}

	return d
}

// Register adds or replaces a handler.
func (d *Dispatcher) Register(handler Handler) {
	if d.handlers == nil {
		d.handlers = make(map[string]Handler)
	}
	d.handlers[handler.Name()] = handler
}

// Dispatch sends an envelope to each requested handler.
func (d *Dispatcher) Dispatch(ctx context.Context, envelope model.Envelope) error {
	if d == nil {
		return errors.New("notification dispatcher is not configured")
	}

	errs := make([]error, 0)
	for _, name := range envelope.Handlers {
		h, ok := d.handlers[name]
		if !ok {
			errs = append(errs, fmt.Errorf("notification handler %q is not configured", name))
			continue
		}

		if err := h.Send(ctx, envelope); err != nil {
			errs = append(errs, fmt.Errorf("notification handler %q failed: %w", name, err))
		}
	}

	return errors.Join(errs...)
}
