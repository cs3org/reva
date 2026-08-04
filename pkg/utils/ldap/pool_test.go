// Copyright 2022 CERN
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

package ldap

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// fakeConn is a minimal ldap.Client double for pool bookkeeping tests. It embeds a nil ldap.Client
// to satisfy the interface; Close and the *Func fields override the methods tests exercise.
type fakeConn struct {
	ldap.Client
	closed bool

	passwordModifyFunc   func(*ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error)
	modifyWithResultFunc func(*ldap.ModifyRequest) (*ldap.ModifyResult, error)
	addFunc              func(*ldap.AddRequest) error
}

func (f *fakeConn) Close() error {
	f.closed = true
	return nil
}

func (f *fakeConn) Add(req *ldap.AddRequest) error {
	if f.addFunc == nil {
		return errors.New("fakeConn: addFunc not set")
	}
	return f.addFunc(req)
}

func (f *fakeConn) PasswordModify(req *ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error) {
	if f.passwordModifyFunc == nil {
		return nil, errors.New("fakeConn: passwordModifyFunc not set")
	}
	return f.passwordModifyFunc(req)
}

func (f *fakeConn) ModifyWithResult(req *ldap.ModifyRequest) (*ldap.ModifyResult, error) {
	if f.modifyWithResultFunc == nil {
		return nil, errors.New("fakeConn: modifyWithResultFunc not set")
	}
	return f.modifyWithResultFunc(req)
}

// newTestPool returns a pool with a fake, network-free dial function and a counter of how many
// times it was called.
func newTestPool(size int, timeout time.Duration) (*ConnPool, *int32) {
	var dialCount int32
	p := NewLDAPPool(Config{PoolSize: size, PoolCheckoutTimeout: timeout}, nil)
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		return &fakeConn{}, nil
	}
	return p, &dialCount
}

func TestConnPoolLazyConstruction(t *testing.T) {
	_, dialCount := newTestPool(2, time.Second)
	if got := atomic.LoadInt32(dialCount); got != 0 {
		t.Fatalf("expected no dials on construction, got %d", got)
	}
}

func TestConnPoolCheckoutReusesReturnedConnection(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	conn, err := p.checkout()
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	p.release(conn, nil)

	conn2, err := p.checkout()
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if conn2 != conn {
		t.Fatalf("expected the returned connection to be reused")
	}
	if got := atomic.LoadInt32(dialCount); got != 1 {
		t.Fatalf("expected exactly 1 dial, got %d", got)
	}
}

func TestConnPoolEvictsUnhealthyConnection(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	conn, err := p.checkout()
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	networkErr := ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
	p.release(conn, networkErr)

	if !conn.(*fakeConn).closed {
		t.Fatalf("expected the unhealthy connection to be closed")
	}

	if _, err := p.checkout(); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if got := atomic.LoadInt32(dialCount); got != 2 {
		t.Fatalf("expected a redial after eviction, got %d dials", got)
	}
}

func TestConnPoolDoRetriesOnceOnNetworkError(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var calls int
	err := p.do(p.read, func(conn ldap.Client) error {
		calls++
		if calls == 1 {
			return ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected do to succeed after one retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected fn to be called twice (initial + 1 retry), got %d", calls)
	}
	if got := atomic.LoadInt32(dialCount); got != 2 {
		t.Fatalf("expected a redial for the retry attempt, got %d dials", got)
	}
}

func TestConnPoolDoGivesUpAfterMaxRetries(t *testing.T) {
	p, _ := newTestPool(2, time.Second)

	networkErr := ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
	var calls int
	err := p.do(p.read, func(conn ldap.Client) error {
		calls++
		return networkErr
	})
	// On exhaustion do returns the last real error, not a synthetic sentinel.
	if !errors.Is(err, networkErr) {
		t.Fatalf("expected the last real error to be surfaced after exhausting retries, got %v", err)
	}
	// newTestPool uses Config{} → RetryMaxCount 0, clamped to 1 → 1 initial + 1 retry.
	if want := 2; calls != want {
		t.Fatalf("expected fn to be called %d times, got %d", want, calls)
	}
}

func TestConnPoolDoDoesNotRetryNonNetworkError(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	nonNetworkErr := errors.New("ldap: invalid credentials")
	var calls int
	err := p.do(p.read, func(conn ldap.Client) error {
		calls++
		return nonNetworkErr
	})
	if !errors.Is(err, nonNetworkErr) {
		t.Fatalf("expected the non-network error to be returned as-is, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected fn to be called exactly once (no retry for non-network errors), got %d", calls)
	}
	if got := atomic.LoadInt32(dialCount); got != 1 {
		t.Fatalf("expected exactly 1 dial (connection reused, no eviction), got %d", got)
	}
}

// TestFinding6_PoolAddNotRetriedOnErrorNetwork asserts a pool write that fails with a post-send
// ErrorNetwork is attempted exactly once and surfaced, not retried (double-apply hazard).
func TestFinding6_PoolAddNotRetriedOnErrorNetwork(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var calls int32
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			addFunc: func(*ldap.AddRequest) error {
				atomic.AddInt32(&calls, 1)
				return ldap.NewError(ldap.ErrorNetwork, errors.New("connection reset during response read"))
			},
		}, nil
	}

	err := p.Add(ldap.NewAddRequest("uid=test,dc=example,dc=org", nil))
	if err == nil || !ldap.IsErrorWithCode(err, ldap.ErrorNetwork) {
		t.Fatalf("expected the network error to be surfaced, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("Finding 6 fix: pool must not retry writes on ErrorNetwork (double-apply). "+
			"expected 1 call, got %d.", got)
	}
}

func TestConnPoolGetLastErrorReturnsNilWithoutCheckout(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	if err := p.GetLastError(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if got := atomic.LoadInt32(dialCount); got != 0 {
		t.Fatalf("expected GetLastError not to check out a connection, got %d dials", got)
	}
}

func TestConnPoolExhaustionTimesOut(t *testing.T) {
	p, _ := newTestPool(1, 50*time.Millisecond)

	if _, err := p.checkout(); err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	start := time.Now()
	_, err := p.checkout()
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("expected ErrPoolExhausted, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("expected checkout to wait for the timeout, only waited %v", elapsed)
	}
}

func TestConnPoolCloseDrainsIdleAndRejectsCheckout(t *testing.T) {
	p, _ := newTestPool(1, time.Second)

	conn, err := p.checkout()
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	p.release(conn, nil)

	if err := p.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !conn.(*fakeConn).closed {
		t.Fatalf("expected the idle connection to be closed by Close")
	}
	if _, err := p.checkout(); !errors.Is(err, errPoolClosed) {
		t.Fatalf("expected errPoolClosed, got %v", err)
	}
}

func TestConnPoolPasswordModify(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var gotReq *ldap.PasswordModifyRequest
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			passwordModifyFunc: func(req *ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error) {
				gotReq = req
				return &ldap.PasswordModifyResult{GeneratedPassword: "generated"}, nil
			},
		}, nil
	}

	req := ldap.NewPasswordModifyRequest("uid=test", "old", "new")
	res, err := p.PasswordModify(req)
	if err != nil {
		t.Fatalf("PasswordModify failed: %v", err)
	}
	if gotReq != req {
		t.Fatalf("expected the request to be forwarded to the underlying connection")
	}
	if res.GeneratedPassword != "generated" {
		t.Fatalf("expected the result to be forwarded from the underlying connection, got %q", res.GeneratedPassword)
	}
}

func TestConnPoolModifyWithResult(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var gotReq *ldap.ModifyRequest
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			modifyWithResultFunc: func(req *ldap.ModifyRequest) (*ldap.ModifyResult, error) {
				gotReq = req
				return &ldap.ModifyResult{}, nil
			},
		}, nil
	}

	req := ldap.NewModifyRequest("uid=test", nil)
	_, err := p.ModifyWithResult(req)
	if err != nil {
		t.Fatalf("ModifyWithResult failed: %v", err)
	}
	if gotReq != req {
		t.Fatalf("expected the request to be forwarded to the underlying connection")
	}
}

// TestConnPoolPasswordModifyNotRetriedOnNetworkError asserts PasswordModify (a write) is not retried
// on a post-send ErrorNetwork, but surfaced after a single attempt.
func TestConnPoolPasswordModifyNotRetriedOnNetworkError(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var calls int
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			passwordModifyFunc: func(req *ldap.PasswordModifyRequest) (*ldap.PasswordModifyResult, error) {
				calls++
				return nil, ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
			},
		}, nil
	}

	req := ldap.NewPasswordModifyRequest("uid=test", "old", "new")
	_, err := p.PasswordModify(req)
	if err == nil || !ldap.IsErrorWithCode(err, ldap.ErrorNetwork) {
		t.Fatalf("expected the network error to be surfaced, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected passwordModifyFunc to be called once (writes are not retried), got %d", calls)
	}
}

// TestConnPoolModifyWithResultNotRetriedOnNetworkError — ModifyWithResult is a
// write; same no-retry-on-ErrorNetwork contract as PasswordModify above.
func TestConnPoolModifyWithResultNotRetriedOnNetworkError(t *testing.T) {
	p, dialCount := newTestPool(2, time.Second)

	var calls int
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			modifyWithResultFunc: func(req *ldap.ModifyRequest) (*ldap.ModifyResult, error) {
				calls++
				return nil, ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
			},
		}, nil
	}

	req := ldap.NewModifyRequest("uid=test", nil)
	_, err := p.ModifyWithResult(req)
	if err == nil || !ldap.IsErrorWithCode(err, ldap.ErrorNetwork) {
		t.Fatalf("expected the network error to be surfaced, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected modifyWithResultFunc to be called once (writes are not retried), got %d", calls)
	}
}

// TestConnPoolHonorsRetryMaxCount: the pool retries a read up to RetryMaxCount times
// (RetryMaxCount+1 total attempts), not the old hardcoded single retry.
func TestConnPoolHonorsRetryMaxCount(t *testing.T) {
	var dialCount int32
	p := NewLDAPPool(Config{PoolSize: 5, PoolCheckoutTimeout: time.Second, RetryMaxCount: 3}, nil)
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		return &fakeConn{}, nil
	}

	var calls int
	err := p.do(p.read, func(conn ldap.Client) error {
		calls++
		return ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
	})
	if err == nil {
		t.Fatalf("expected an error after exhausting retries")
	}
	if want := 4; calls != want { // 1 initial + 3 retries
		t.Fatalf("expected fn to be called %d times, got %d", want, calls)
	}
}

// TestConnPoolAppliesBackoff: with a BaseDelay set, the pool sleeps between retries with a
// growing backoff, via the injectable sleepFn.
func TestConnPoolAppliesBackoff(t *testing.T) {
	p := NewLDAPPool(Config{
		PoolSize:            5,
		PoolCheckoutTimeout: time.Second,
		RetryMaxCount:       2,
		RetryBaseDelay:      10 * time.Millisecond,
		RetryMaxDelay:       200 * time.Millisecond,
	}, nil)
	p.dial = func(Config) (ldap.Client, error) { return &fakeConn{}, nil }
	var slept []time.Duration
	p.sleepFn = func(d time.Duration) { slept = append(slept, d) }

	_ = p.do(p.read, func(conn ldap.Client) error {
		return ldap.NewError(ldap.ErrorNetwork, errors.New("boom"))
	})

	if len(slept) != 2 { // 2 retries → 2 sleeps
		t.Fatalf("expected 2 backoff sleeps, got %d", len(slept))
	}
	for _, d := range slept {
		if d <= 0 || d > 200*time.Millisecond {
			t.Fatalf("sleep %v out of expected (0, MaxDelay] range", d)
		}
	}
}

// TestConnPoolWriteRetriesPreSendNetworkError: a pooled write that fails with a pre-send
// ErrorNetwork (stale idle connection) is retried on a fresh connection.
func TestConnPoolWriteRetriesPreSendNetworkError(t *testing.T) {
	var dialCount int32
	p := NewLDAPPool(Config{PoolSize: 5, PoolCheckoutTimeout: time.Second}, nil)
	var calls int32
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		return &fakeConn{
			addFunc: func(*ldap.AddRequest) error {
				if atomic.AddInt32(&calls, 1) == 1 {
					return ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: connection closed"))
				}
				return nil
			},
		}, nil
	}

	if err := p.Add(ldap.NewAddRequest("uid=test,dc=example,dc=org", nil)); err != nil {
		t.Fatalf("expected Add to succeed after retry, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected Add to be retried once (pre-send ErrorNetwork), got %d calls", got)
	}
	if got := atomic.LoadInt32(&dialCount); got != 2 {
		t.Fatalf("expected a redial for the retry, got %d dials", got)
	}
}

// TestConnPoolWriteDoesNotRetryPostSendNetworkError: a pooled write that fails with a post-send
// ErrorNetwork must not be retried — the mutation may already have applied.
func TestConnPoolWriteDoesNotRetryPostSendNetworkError(t *testing.T) {
	p, dialCount := newTestPool(5, time.Second)
	var calls int32
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(dialCount, 1)
		return &fakeConn{
			addFunc: func(*ldap.AddRequest) error {
				atomic.AddInt32(&calls, 1)
				return ldap.NewError(ldap.ErrorNetwork, errors.New("ldap: response channel closed"))
			},
		}, nil
	}

	err := p.Add(ldap.NewAddRequest("uid=test,dc=example,dc=org", nil))
	if err == nil || !ldap.IsErrorWithCode(err, ldap.ErrorNetwork) {
		t.Fatalf("expected the network error to be surfaced, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected Add not to be retried on a post-send ErrorNetwork, got %d calls", got)
	}
}

// TestConnPoolWriteRetriesSendFailedError asserts a pooled write failing with a codeless failed
// conn.Write is retried on a fresh connection (matched by isSendFailedErr, not by result code).
func TestConnPoolWriteRetriesSendFailedError(t *testing.T) {
	var dialCount int32
	p := NewLDAPPool(Config{PoolSize: 5, PoolCheckoutTimeout: time.Second}, nil)
	var calls int32
	p.dial = func(Config) (ldap.Client, error) {
		atomic.AddInt32(&dialCount, 1)
		return &fakeConn{
			addFunc: func(*ldap.AddRequest) error {
				if atomic.AddInt32(&calls, 1) == 1 {
					return errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe")
				}
				return nil
			},
		}, nil
	}

	if err := p.Add(ldap.NewAddRequest("uid=test,dc=example,dc=org", nil)); err != nil {
		t.Fatalf("expected Add to succeed after retry, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected Add to be retried once (failed conn.Write), got %d calls", got)
	}
	if got := atomic.LoadInt32(&dialCount); got != 2 {
		t.Fatalf("expected a redial for the retry, got %d dials", got)
	}
}

// TestConnPoolReleaseEvictsOnSendFailedError asserts a connection whose conn.Write failed is closed
// and discarded, not returned to the idle pool (the codeless error needs isSendFailedErr to evict).
func TestConnPoolReleaseEvictsOnSendFailedError(t *testing.T) {
	p, _ := newTestPool(2, time.Second)

	conn := &fakeConn{}
	if err := p.sem.Acquire(context.Background(), 1); err != nil {
		t.Fatalf("failed to reserve a pool slot: %v", err)
	}
	p.release(conn, errors.New("unable to send request: write tcp 127.0.0.1:1->127.0.0.1:2: write: broken pipe"))

	if !conn.closed {
		t.Fatal("expected the connection to be closed and discarded after a failed conn.Write")
	}
	select {
	case c := <-p.idle:
		t.Fatalf("expected no connection returned to the idle pool, got %v", c)
	default:
	}
}

func TestConnPoolConcurrentCheckoutRelease(t *testing.T) {
	p, dialCount := newTestPool(4, time.Second)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := p.checkout()
			if err != nil {
				t.Errorf("checkout failed: %v", err)
				return
			}
			p.release(conn, nil)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(dialCount); got > 4 {
		t.Fatalf("expected at most 4 dials (pool size), got %d", got)
	}
}
