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

package model

import "time"

const (
	TypeDirect      = "direct"
	TypeAccumulated = "accumulated"
)

// AccumulationPolicy controls how accumulated notifications are grouped.
type AccumulationPolicy struct {
	WindowSeconds int `json:"window_seconds" mapstructure:"window_seconds"`
	MaxItems      int `json:"max_items" mapstructure:"max_items"`
}

// SendRequest is the internal request shape used by the gateway PublishEvent
// implementation before the request is published to a backend.
type SendRequest struct {
	EventType      string         `json:"event_type"`
	SubmittingUser string         `json:"submitting_user"`
	Sender         string         `json:"sender,omitempty"`
	Recipients     []string       `json:"recipients"`
	TemplateData   map[string]any `json:"template_data,omitempty"`
}

// Envelope is the durable notification message sent through NATS and stored in
// SQL for accumulated notifications.
type Envelope struct {
	ID             string         `json:"id"`
	EventType      string         `json:"event_type"`
	SubmittingUser string         `json:"submitting_user"`
	Sender         string         `json:"sender,omitempty"`
	Recipients     []string       `json:"recipients"`
	TemplateData   map[string]any `json:"template_data,omitempty"`
	SubmittedAt    time.Time      `json:"submitted_at"`

	// The following fields are resolved by the notification worker from event
	// rules before dispatch or accumulation. They are not accepted from
	// producers.
	Type         string             `json:"type,omitempty"`
	DedupKey     string             `json:"dedup_key,omitempty"`
	Handlers     []string           `json:"handlers,omitempty"`
	TemplateName string             `json:"template_name,omitempty"`
	Accumulation AccumulationPolicy `json:"accumulation,omitempty"`
}

// HandlerRule configures one handler for a notification event.
type HandlerRule struct {
	TemplateName string `json:"template_name,omitempty" mapstructure:"template_name"`
}

// EventRule configures how one notification event type is delivered.
type EventRule struct {
	Type             string                 `json:"type" mapstructure:"type"`
	DedupKeyTemplate string                 `json:"dedup_key_template,omitempty" mapstructure:"dedup_key_template"`
	Handlers         map[string]HandlerRule `json:"handlers" mapstructure:"handlers"`
	Accumulation     AccumulationPolicy     `json:"accumulation,omitempty" mapstructure:"accumulation"`
}

// Bucket describes the current SQL-backed accumulation state for a dedup key.
type Bucket struct {
	DedupKey      string
	FirstSeen     time.Time
	LatestSeen    time.Time
	FlushAfter    time.Time
	WindowSeconds int
	MaxItems      int
	ItemCount     int
	Status        string
	LeaseOwner    string
	LeaseUntil    time.Time
}
