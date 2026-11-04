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

// Package reminders tells users about a condition on their data, such as a
// public link that expires soon or a space that is almost full. A kind finds
// the subjects in the condition and builds the notification. The framework
// remembers what it sent, so that a recipient is told once per level.
package reminders

import (
	"context"
	"encoding/json"
	"iter"
)

// Reminder is one kind of reminder.
type Reminder interface {
	// Event is the notification event type that the kind publishes.
	Event() string
	// Scan yields every subject whose condition holds now. The framework
	// closes the episode of a subject that a scan does not yield, so a scan
	// must be complete. Yield an error for a subject that the kind cannot
	// evaluate: then the framework closes nothing in that run.
	Scan(ctx context.Context) iter.Seq2[Item, error]
	// Notice builds the template data of one notification from the due items
	// of one recipient, highest level first, then by subject.
	Notice(ctx context.Context, to Recipient, items iter.Seq[Item]) (map[string]any, error)
}

// Item is a condition that holds on a subject.
type Item struct {
	// Subject identifies the condition across runs. A new subject starts a
	// new episode.
	Subject string
	// Level is the urgency, higher is more urgent. A recipient is told when
	// the level is higher than the highest level sent in the episode. Level 0
	// keeps the episode open and tells nobody.
	Level     int
	Recipient Recipient
	// Data is kept in the ledger with the claim and given back to Notice.
	Data json.RawMessage
}

// Recipient is a user or an e-group.
type Recipient struct {
	Name  string
	Group bool
}

// Config is the part of the configuration of a kind that the framework reads.
type Config struct {
	// Schedule runs the kind periodically, for example "@daily". Empty runs
	// it only on demand.
	Schedule   string `mapstructure:"schedule"`
	DryRun     bool   `mapstructure:"dry_run"`
	RunOnStart bool   `mapstructure:"run_on_start"`
}
