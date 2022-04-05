// Copyright 2018-2022 CERN
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

package events

import (
	"encoding/json"
)

// GroupCreated is emitted when a group was created
type GroupCreated struct {
	GroupID string
}

// Unmarshal to fulfill umarshaller interface
func (GroupCreated) Unmarshal(v []byte) (interface{}, error) {
	e := GroupCreated{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// GroupDeleted is emitted when a group was deleted
type GroupDeleted struct {
	GroupID string
}

// Unmarshal to fulfill umarshaller interface
func (GroupDeleted) Unmarshal(v []byte) (interface{}, error) {
	e := GroupDeleted{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// GroupMemberAdded is emitted when a user was added to a group
type GroupMemberAdded struct {
	GroupID string
	UserID  string
}

// Unmarshal to fulfill umarshaller interface
func (GroupMemberAdded) Unmarshal(v []byte) (interface{}, error) {
	e := GroupMemberAdded{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// GroupMemberRemoved is emitted when a user was removed from a group
type GroupMemberRemoved struct {
	GroupID string
	UserID  string
}

// Unmarshal to fulfill umarshaller interface
func (GroupMemberRemoved) Unmarshal(v []byte) (interface{}, error) {
	e := GroupMemberRemoved{}
	err := json.Unmarshal(v, &e)
	return e, err
}
