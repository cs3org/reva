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

// Package model holds the GORM model for the SQL job status store.
package model

import (
	"time"

	"gorm.io/datatypes"
)

// Run is the persisted status of a single run, keyed by RunID.
type Run struct {
	RunID      string `gorm:"primaryKey;size:255"`
	Job        string `gorm:"index;size:255"`
	State      string `gorm:"index;size:32"`
	Attempt    int
	EnqueuedAt time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	LastError  string         `gorm:"type:text"`
	Result     datatypes.JSON `gorm:"type:json"`

	// Owner is the username the run was created for, empty for an internal run.
	// It is indexed so a user's runs can be listed.
	Owner string `gorm:"size:255;index:idx_owner"`
}

// TableName sets the table name explicitly so it does not collide with other
// "runs" tables.
func (Run) TableName() string {
	return "job_runs"
}
