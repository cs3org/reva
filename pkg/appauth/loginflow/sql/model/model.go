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

import (
	"time"

	"gorm.io/gorm"
)

// Flow is the persisted state of one login flow enrolment. gorm.Model gives
// ID, CreatedAt, UpdatedAt and DeletedAt; DeletedAt drives the soft-delete that
// marks a flow consumed, denied or expired.
type Flow struct {
	gorm.Model
	LoginHash  []byte     `gorm:"uniqueIndex;size:32;not null"` // SHA256(logintoken)
	PollHash   []byte     `gorm:"uniqueIndex;size:32;not null"` // SHA256(polltoken)
	ClientID   string     `gorm:"uniqueIndex;size:36;not null"` // UUIDv4
	UserAgent  string     `gorm:"size:512;not null"`
	ExpiresAt  time.Time  `gorm:"index;not null"`
	ApprovedAt *time.Time // null = PENDING; non-null = APPROVED
	UserID     string     `gorm:"size:64;index"` // populated on approval
	Username   string     `gorm:"size:255"`      // populated on approval
	DeviceName string     `gorm:"size:64"`       // populated on approval
}
