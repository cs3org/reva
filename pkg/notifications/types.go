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

package notifications

import "time"

const (
	TypeDirect      = "direct"
	TypeAccumulated = "accumulated"
)

// AccumulationPolicy controls how accumulated notifications are grouped.
type AccumulationPolicy struct {
	WindowSeconds int `json:"window_seconds"`
	MaxItems      int `json:"max_items"`
}

// SendRequest is the internal request shape used by the gateway SendNotification
// implementation before the request is published to a backend.
type SendRequest struct {
	Type           string             `json:"type"`
	DedupKey       string             `json:"dedup_key,omitempty"`
	SubmittingUser string             `json:"submitting_user"`
	Sender         string             `json:"sender,omitempty"`
	Recipients     []string           `json:"recipients"`
	Handlers       []string           `json:"handlers"`
	TemplateName   string             `json:"template_name,omitempty"`
	TemplateData   map[string]any     `json:"template_data,omitempty"`
	Accumulation   AccumulationPolicy `json:"accumulation,omitempty"`
}

// Envelope is the durable notification message sent through NATS and stored in
// SQL for accumulated notifications.
type Envelope struct {
	ID             string             `json:"id"`
	Type           string             `json:"type"`
	DedupKey       string             `json:"dedup_key,omitempty"`
	SubmittingUser string             `json:"submitting_user"`
	Sender         string             `json:"sender,omitempty"`
	Recipients     []string           `json:"recipients"`
	Handlers       []string           `json:"handlers"`
	TemplateName   string             `json:"template_name,omitempty"`
	TemplateData   map[string]any     `json:"template_data,omitempty"`
	Accumulation   AccumulationPolicy `json:"accumulation,omitempty"`
	SubmittedAt    time.Time          `json:"submitted_at"`
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
