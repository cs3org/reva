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

package user

// BlockedUsers is a set containing all the blocked users
type BlockedUsers map[string]struct{}

// NewBlockedUsersSet creates a new set of blocked users from a list
func NewBlockedUsersSet(users []string) BlockedUsers {
	s := make(map[string]struct{})
	for _, u := range users {
		s[u] = struct{}{}
	}
	return s
}

// IsBlocked returns true if the user is blocked
func (b BlockedUsers) IsBlocked(user string) bool {
	_, ok := b[user]
	return ok
}
