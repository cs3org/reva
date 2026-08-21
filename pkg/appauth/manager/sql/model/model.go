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

// AppPassword is a stored application password. The secret itself is never
// stored; only its SHA-256 hash is. The 256-bit token entropy makes a
// single SHA-256 enough: a DB dump holds no usable credential.
type AppPassword struct {
	ID         uint       `gorm:"primaryKey"`
	UserID     string     `gorm:"size:64;index;not null"`       // opaque user id
	TokenHash  []byte     `gorm:"uniqueIndex;size:32;not null"` // SHA256(token)
	Label      string     `gorm:"size:255"`
	ScopeJSON  []byte     // JSON-encoded map[string]*auth.Scope
	Ctime      time.Time  `gorm:"not null"`
	Utime      time.Time  `gorm:"not null"` // last seen; buffered write
	Expiration *time.Time // nil = never expires
}
