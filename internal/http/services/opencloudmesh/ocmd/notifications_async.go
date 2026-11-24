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

	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/cs3org/reva/v3/pkg/prom/registry"
	"github.com/prometheus/client_golang/prometheus"
)

// maxInFlightNotifications limits the attempts that run at the same time, so
// that a large change to many shares of a slow peer does not keep a goroutine
// and a connection for each share.
const maxInFlightNotifications = 64

// FailureSaturated is the failure of a notification that was not sent because
// too many attempts were running. It is not tried again.
const FailureSaturated SendFailure = "saturated"

var saturatedNotifications = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "reva_ocm_notifications_saturated_total",
	Help: "Number of outgoing OCM notifications dropped because all send slots were in use.",
})

func init() {
	registry.Register("ocm_notifications", func(context.Context, map[string]any) ([]prometheus.Collector, error) {
		return []prometheus.Collector{saturatedNotifications}, nil
	})
}

// AsyncNotifier sends notifications in the background, so that a slow peer
// does not delay the caller. The service that owns it must Close it.
type AsyncNotifier struct {
	Client *OCMClient
	// Key is the owning service's notification signing key. It is passed to
	// every attempt explicitly; the notifier never stores credentials in a
	// notification payload or appctx.
	Key *httpsig.Key

	mu       sync.Mutex
	closed   bool
	inFlight int
	lifetime context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewNotification starts one attempt and returns at once. The attempt keeps
// the values of ctx, but only Close cancels it: the change it reports is
// already committed.
func (a *AsyncNotifier) NewNotification(ctx context.Context, domain string, n *OutgoingNotification) SendResult {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return SendResult{Outcome: SendPostFailed, Failure: FailureCanceled}
	}
	if a.inFlight >= maxInFlightNotifications {
		a.mu.Unlock()
		saturatedNotifications.Inc()
		return SendResult{Outcome: SendPostFailed, Failure: FailureSaturated}
	}
	a.inFlight++
	if a.lifetime == nil {
		a.lifetime, a.cancel = context.WithCancel(context.Background())
	}
	sendCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(a.lifetime, cancel)
	a.wg.Add(1)
	a.mu.Unlock()
	go func() {
		defer a.wg.Done()
		defer func() {
			a.mu.Lock()
			a.inFlight--
			a.mu.Unlock()
		}()
		defer cancel()
		defer stop()
		a.Client.NewNotification(sendCtx, domain, n, a.Key)
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
