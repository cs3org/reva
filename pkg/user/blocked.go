// Copyright 2018-2024 CERN
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

import "sync"

// BlockedUsers is a concurrency-safe set of blocked usernames, consulted on
// the authentication hot path.
type BlockedUsers struct {
	mu    sync.RWMutex
	users map[string]struct{}
}

// NewBlockedUsersSet creates a new set of blocked users from a list.
func NewBlockedUsersSet(users []string) *BlockedUsers {
	s := &BlockedUsers{users: make(map[string]struct{}, len(users))}
	for _, u := range users {
		s.users[u] = struct{}{}
	}
	return s
}

// IsBlocked returns true if the user is blocked.
func (b *BlockedUsers) IsBlocked(user string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.users[user]
	return ok
}

// Block adds a user to the set.
func (b *BlockedUsers) Block(user string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.users[user] = struct{}{}
}

// Unblock removes a user from the set.
func (b *BlockedUsers) Unblock(user string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.users, user)
}

// Add seeds additional blocked users.
func (b *BlockedUsers) Add(users ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, u := range users {
		b.users[u] = struct{}{}
	}
}

// List returns the currently blocked usernames.
func (b *BlockedUsers) List() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.users))
	for u := range b.users {
		out = append(out, u)
	}
	return out
}

var (
	sharedMu      sync.Mutex
	sharedBlocked *BlockedUsers
)

// SharedBlockedUsers returns the process-wide blocked set, created on first
// use. The auth interceptor and authprovider seed it from config.
func SharedBlockedUsers() *BlockedUsers {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if sharedBlocked == nil {
		sharedBlocked = NewBlockedUsersSet(nil)
	}
	return sharedBlocked
}
