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
	pool, _, err := NewUserThreadPool(UserThreadPoolConfig{})
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
