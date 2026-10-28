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

//go:build linux

package cephmount

import (
	"context"
	"sync"
	"testing"
	"time"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserThreadCleanupKeepsBusyThread(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{ThreadTTL: time.Nanosecond})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	// The request runs for longer than the TTL, as a large upload can
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, err := pool.ExecuteOnUserThread(ctx, user, func() (any, error) {
			close(started)
			<-release
			return nil, nil
		})
		done <- err
	}()
	<-started

	pool.cleanupExpiredThreads()
	close(release)
	require.NoError(t, <-done)
}

func TestUserThreadAfterCancelledRequest(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	// The caller stops waiting while the request runs, as when a client cancels a slow stat
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	_, err = pool.ExecuteOnUserThread(ctx, user, func() (any, error) {
		cancel()
		<-release
		return "slow", nil
	})
	require.ErrorIs(t, err, context.Canceled)
	close(release)

	for i := range 3 {
		res, err := pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
			return i, nil
		})
		require.NoError(t, err)
		assert.Equal(t, i, res)
	}
}

func TestUserThreadParallelRequests(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Go(func() {
			res, err := pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
				// Different durations change the order in which the callers wait
				time.Sleep(time.Duration(i%3) * 100 * time.Microsecond)
				return i, nil
			})
			assert.NoError(t, err)
			assert.Equal(t, i, res)
		})
	}
	wg.Wait()
}

func TestUserThreadSkipsCancelledRequest(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{MaxThreadsPerUser: 1})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	// Keep the thread busy, so that the next request stays in the queue
	started, release := make(chan struct{}), make(chan struct{})
	go pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
		close(started)
		<-release
		return nil, nil
	})
	<-started

	ran := false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = pool.ExecuteOnUserThread(ctx, user, func() (any, error) {
		ran = true
		return nil, nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)

	// Requests run in order, so when this request returns, the thread has handled the cancelled request
	_, err = pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
		return nil, nil
	})
	require.NoError(t, err)
	assert.False(t, ran)
}

func TestUserThreadCapStopsIdleThread(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{MaxThreads: 1})
	require.NoError(t, err)
	defer pool.Shutdown()
	einstein := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}
	marie := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "marie"}, Username: "marie", UidNumber: 4243, GidNumber: 4243}
	noop := func() (any, error) { return nil, nil }

	_, err = pool.ExecuteOnUserThread(context.Background(), einstein, noop)
	require.NoError(t, err)

	// The thread of einstein is idle, thus marie gets its place
	_, err = pool.ExecuteOnUserThread(context.Background(), marie, noop)
	require.NoError(t, err)
}

func TestUserThreadCapRefusesWhenAllThreadsRun(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{MaxThreads: 1})
	require.NoError(t, err)
	defer pool.Shutdown()
	einstein := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}
	marie := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "marie"}, Username: "marie", UidNumber: 4243, GidNumber: 4243}
	noop := func() (any, error) { return nil, nil }

	// The caller stops waiting, but the request continues to run on the thread
	ctx, cancel := context.WithCancel(context.Background())
	release := make(chan struct{})
	_, err = pool.ExecuteOnUserThread(ctx, einstein, func() (any, error) {
		cancel()
		<-release
		return nil, nil
	})
	require.ErrorIs(t, err, context.Canceled)

	_, err = pool.ExecuteOnUserThread(context.Background(), marie, noop)
	require.Error(t, err)

	// The thread becomes idle shortly after the request ends
	close(release)
	require.Eventually(t, func() bool {
		_, err := pool.ExecuteOnUserThread(context.Background(), marie, noop)
		return err == nil
	}, time.Second, time.Millisecond)
}

func TestUserThreadRunsRequestsOfOneUserInParallel(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	// The first request waits until the second request has run
	started, ran := make(chan struct{}), make(chan struct{})
	parallel := make(chan bool, 1)
	go pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
		close(started)
		select {
		case <-ran:
			parallel <- true
		case <-time.After(time.Second):
			parallel <- false
		}
		return nil, nil
	})
	<-started

	_, err = pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
		close(ran)
		return nil, nil
	})
	require.NoError(t, err)
	assert.True(t, <-parallel)
}

func TestUserThreadWorkerLimit(t *testing.T) {
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{MaxThreadsPerUser: 2})
	require.NoError(t, err)
	defer pool.Shutdown()
	user := &userv1beta1.User{Id: &userv1beta1.UserId{OpaqueId: "einstein"}, Username: "einstein", UidNumber: 4242, GidNumber: 4242}

	// Two requests hold both workers of the user
	var started sync.WaitGroup
	started.Add(2)
	release := make(chan struct{})
	defer close(release)
	for range 2 {
		go pool.ExecuteOnUserThread(context.Background(), user, func() (any, error) {
			started.Done()
			<-release
			return nil, nil
		})
	}
	started.Wait()

	// No third worker starts, thus the request waits until its deadline
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = pool.ExecuteOnUserThread(ctx, user, func() (any, error) {
		return nil, nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
