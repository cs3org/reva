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

package sql

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/pkg/appauth/loginflow"
	"github.com/cs3org/reva/v3/pkg/errtypes"
)

func newTestManager(t *testing.T) loginflow.Manager {
	t.Helper()
	m, err := New(context.Background(), map[string]any{
		"db_engine": "sqlite",
		"db_name":   filepath.Join(t.TempDir(), "loginflow.db"),
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func pendingAuthorization() *loginflow.ClientAuthorization {
	return &loginflow.ClientAuthorization{
		LoginHash: []byte("login-hash-0000000000000000000000"),
		PollHash:  []byte("poll-hash-00000000000000000000000"),
		ClientID:  "client-1",
		UserAgent: "mirall/3.16.0 (Linux)",
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func TestCreateAndGet(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := m.GetByLogin(ctx, ca.LoginHash)
	if err != nil {
		t.Fatalf("get by login: %v", err)
	}
	if got.ClientID != ca.ClientID || got.Approved() {
		t.Fatalf("unexpected authorization: %+v", got)
	}
	if _, err := m.GetByPoll(ctx, ca.PollHash); err != nil {
		t.Fatalf("get by poll: %v", err)
	}
	if _, err := m.GetByLogin(ctx, []byte("nope")); !isNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
}

func TestApproveIsSingleShot(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if err := m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop"); !isConflict(err) {
		t.Fatalf("second approve should conflict, got %v", err)
	}

	got, _ := m.GetByLogin(ctx, ca.LoginHash)
	if !got.Approved() || got.Username != "jdoe" || got.DeviceName != "laptop" {
		t.Fatalf("approval not recorded: %+v", got)
	}
}

func TestConcurrentApproveOneWinner(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	wins := make(chan bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wins <- m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop") == nil
		}()
	}
	wg.Wait()
	close(wins)

	won := 0
	for w := range wins {
		if w {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("want exactly one approve winner, got %d", won)
	}
}

func TestConsumeRequiresApproval(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := m.Consume(ctx, ca.PollHash); !isNotFound(err) {
		t.Fatalf("consume of pending authorization should be not found, got %v", err)
	}

	if err := m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	got, err := m.Consume(ctx, ca.PollHash)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if got.Username != "jdoe" || got.DeviceName != "laptop" || got.UserID != "uid" {
		t.Fatalf("consumed authorization missing fields: %+v", got)
	}

	// Second consume finds nothing: the row is soft-deleted.
	if _, err := m.Consume(ctx, ca.PollHash); !isNotFound(err) {
		t.Fatalf("second consume should be not found, got %v", err)
	}
	if _, err := m.GetByLogin(ctx, ca.LoginHash); !isNotFound(err) {
		t.Fatalf("consumed authorization should not be live, got %v", err)
	}
}

func TestConcurrentConsumeOneWinner(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	const n = 8
	var wg sync.WaitGroup
	wins := make(chan bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Consume(ctx, ca.PollHash)
			wins <- err == nil
		}()
	}
	wg.Wait()
	close(wins)

	won := 0
	for w := range wins {
		if w {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("want exactly one consume winner, got %d", won)
	}
}

func TestDenySoftDeletes(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Deny(ctx, ca.LoginHash); err != nil {
		t.Fatalf("deny: %v", err)
	}
	if _, err := m.GetByLogin(ctx, ca.LoginHash); !isNotFound(err) {
		t.Fatalf("denied authorization should not be live, got %v", err)
	}
}

func TestExpiredAuthorizationIsReturnedButNotApprovable(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	ca := pendingAuthorization()
	ca.ExpiresAt = time.Now().Add(-time.Minute)
	if err := m.Create(ctx, ca); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := m.GetByLogin(ctx, ca.LoginHash)
	if err != nil {
		t.Fatalf("get expired: %v", err)
	}
	if !got.Expired() {
		t.Fatalf("authorization should report expired")
	}
	if err := m.Approve(ctx, ca.LoginHash, "uid", "jdoe", "laptop"); !isConflict(err) {
		t.Fatalf("approve of expired authorization should conflict, got %v", err)
	}
}

func isNotFound(err error) bool {
	_, ok := err.(errtypes.NotFound)
	return ok
}

func isConflict(err error) bool {
	_, ok := err.(errtypes.Conflict)
	return ok
}
