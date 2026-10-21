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

package ocmd

import (
	"context"
	"sync"

	"github.com/cs3org/reva/v3/pkg/appctx"
)

// AsyncNotifier sends notifications in the background, so that a slow peer
// does not delay the caller. The service that owns it must Close it.
type AsyncNotifier struct {
	Client *OCMClient

	mu       sync.Mutex
	closed   bool
	lifetime context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewNotification starts one attempt and returns at once. The attempt keeps
// the values of ctx, but only Close cancels it: the change it reports is
// already committed.
func (a *AsyncNotifier) NewNotification(ctx context.Context, domain string, n *OutgoingNotification) SendResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		appctx.GetLogger(ctx).Warn().Str("type", n.Type).Msg("OCM notification not sent: service is closing")
		return SendResult{Outcome: SendPostFailed, Failure: FailureCanceled}
	}
	if a.lifetime == nil {
		a.lifetime, a.cancel = context.WithCancel(context.Background())
	}
	sendCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(a.lifetime, cancel)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer cancel()
		defer stop()
		a.Client.NewNotification(sendCtx, domain, n)
	}()
	return SendResult{}
}

// Close stops new attempts, cancels the running ones and waits for them.
func (a *AsyncNotifier) Close() error {
	a.mu.Lock()
	a.closed = true
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	return nil
}
