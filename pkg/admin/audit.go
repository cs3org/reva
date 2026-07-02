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

// Package admin holds the primitives specific to the reva Admin API: the
// audit helper. The invocation framework lives in pkg/invoke.
package admin

import (
	"context"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

// AuditEvent is one admin action worth recording: who did what, against what,
// and how it turned out.
type AuditEvent struct {
	// Action is the operation, e.g. "request_admin", "invoke".
	Action string
	// Actor is the username of the caller.
	Actor string
	// Target is the affected user/service/node, when there is one.
	Target string
	// Kind is the invocation kind or an action sub-class, when relevant.
	Kind string
	// Granted records the outcome of a gate decision.
	Granted bool
	// Err is set when the action failed or was denied.
	Err error
	// Fields carries any extra structured context.
	Fields map[string]string
}

// Audit emits a structured audit event (audit=true) on the context logger.
// Denials and failures are logged at error level, successes at info.
func Audit(ctx context.Context, ev AuditEvent) {
	log := appctx.GetLogger(ctx)
	e := log.Info()
	if ev.Err != nil {
		e = log.Error().Err(ev.Err)
	}
	e = e.Bool("audit", true).
		Str("action", ev.Action).
		Str("actor", ev.Actor).
		Bool("granted", ev.Granted)
	if ev.Target != "" {
		e = e.Str("target", ev.Target)
	}
	if ev.Kind != "" {
		e = e.Str("kind", ev.Kind)
	}
	for k, v := range ev.Fields {
		e = e.Str(k, v)
	}
	e.Msg("admin audit")
}
