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

package ocmd

import (
	"context"
	"encoding/json"
	"hash/crc32"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmclient "github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rs/zerolog"
)

// gatedPeer holds every discovery request until it is released or the
// client goes away, and counts the notification POSTs it receives.
type gatedPeer struct {
	srv       *httptest.Server
	release   chan struct{}
	entered   chan struct{}
	posted    chan struct{}
	posts     atomic.Int32
	discovers atomic.Int32
}

func newGatedPeer(t *testing.T) *gatedPeer {
	p := &gatedPeer{release: make(chan struct{}), entered: make(chan struct{}, 64), posted: make(chan struct{}, 64)}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			p.discovers.Add(1)
			p.entered <- struct{}{}
			select {
			case <-p.release:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
				Enabled: true, APIVersion: "1.4.0", Endpoint: p.srv.URL + "/ocm", Capabilities: []string{"notifications"},
			})
		case "/ocm/notifications":
			p.posts.Add(1)
			p.posted <- struct{}{}
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func wait(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestAsyncNotifierSerializesCurrentPermissionSnapshots(t *testing.T) {
	p := newGatedPeer(t)
	client := NewClientWithConfig(ocmclient.TransportConfig{Timeout: 5 * time.Second, AllowLoopback: true})
	a := &AsyncNotifier{Client: client, Key: newSigningKey(t)}
	t.Cleanup(func() { _ = a.Close() })
	host := strings.TrimPrefix(p.srv.URL, "http://")
	send := func(current bool, permissions []string, checked chan struct{}) {
		t.Helper()
		n, err := (ShareStateNotification{
			Type: NotificationShareChangePermission, SenderDomain: "cernbox.example.org",
			ResourceType: "file", ShareType: "user", ProviderID: "same-grant", Permissions: permissions,
		}).Build()
		if err != nil {
			t.Fatal(err)
		}
		signal := sync.OnceFunc(func() { close(checked) })
		n.StillCurrent = func(context.Context) bool {
			signal()
			return current
		}
		if result := a.NewNotification(context.Background(), host, n); result != (SendResult{}) {
			t.Fatalf("send admission: %+v", result)
		}
	}

	send(true, []string{"read", "write"}, make(chan struct{}))
	wait(t, p.entered, "first discovery")
	stale, latest := make(chan struct{}), make(chan struct{})
	send(false, []string{"read"}, stale)
	send(true, []string{}, latest)
	select {
	case <-latest:
		t.Fatal("the newer snapshot overtook the in-flight send")
	case <-time.After(50 * time.Millisecond):
	}
	close(p.release)
	wait(t, p.posted, "first POST")
	wait(t, p.posted, "latest POST")
	wait(t, stale, "stale snapshot check")
	joined := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(joined)
	}()
	wait(t, joined, "notification workers")
	if d, n := p.discovers.Load(), p.posts.Load(); d != 2 || n != 2 {
		t.Fatalf("discovers %d, posts %d", d, n)
	}
}

func TestAsyncNotifierStateNoticesIsolatePeersWithSameGrantID(t *testing.T) {
	releaseSlow := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(releaseSlow) })
	defer unblock()
	slowEntered := make(chan struct{}, 1)
	slowPosted := make(chan struct{}, 1)
	fastPosted := make(chan struct{}, 1)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/ocm":
			route := "/fast"
			if strings.HasPrefix(r.Host, "127.0.0.1:") {
				route = "/slow"
				slowEntered <- struct{}{}
				select {
				case <-releaseSlow:
				case <-r.Context().Done():
					return
				}
			}
			_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
				Enabled: true, APIVersion: "1.4.0",
				Endpoint:     "http://" + r.Context().Value(http.LocalAddrContextKey).(net.Addr).String() + route,
				Capabilities: []string{"notifications"},
			})
		case "/slow/notifications":
			w.WriteHeader(http.StatusCreated)
			slowPosted <- struct{}{}
		case "/fast/notifications":
			w.WriteHeader(http.StatusCreated)
			fastPosted <- struct{}{}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(peer.Close)

	host := peer.Listener.Addr().String()
	_, port, err := net.SplitHostPort(host)
	if err != nil {
		t.Fatal(err)
	}
	otherHost := net.JoinHostPort("localhost", port)
	grantID := "same-grant"
	for range 32 {
		first := crc32.ChecksumIEEE([]byte(host+"\x00"+grantID)) % maxInFlightNotifications
		second := crc32.ChecksumIEEE([]byte(otherHost+"\x00"+grantID)) % maxInFlightNotifications
		if first != second {
			break
		}
		grantID += "x"
	}
	if len(grantID) == len("same-grant")+32 {
		t.Fatal("peer fixtures could not be assigned different send stripes")
	}
	client := NewClientWithConfig(ocmclient.TransportConfig{Timeout: time.Minute, AllowLoopback: true})
	a := &AsyncNotifier{Client: client, Key: newSigningKey(t)}
	t.Cleanup(func() { _ = a.Close() })
	send := func(domain string) {
		t.Helper()
		n, err := (ShareStateNotification{
			Type: NotificationShareAccepted, SenderDomain: "cernbox.example.org",
			ResourceType: "file", ShareType: "user", ProviderID: grantID,
		}).Build()
		if err != nil {
			t.Fatal(err)
		}
		n.StateNotice = &share.StateNotice{StillCurrent: func() bool { return true }, Release: func() {}}
		if result := a.NewNotification(context.Background(), domain, n); result != (SendResult{}) {
			t.Fatalf("send admission: %+v", result)
		}
	}

	send(host)
	wait(t, slowEntered, "slow peer discovery")
	send(otherHost)
	select {
	case <-fastPosted:
	case <-time.After(3 * time.Second):
		t.Fatal("independent peer did not receive the state notice while slow discovery was held")
	}
	unblock()
	wait(t, slowPosted, "slow peer POST")
}

func TestAsyncNotifierCloseCancelsAdmittedSend(t *testing.T) {
	p := newGatedPeer(t)
	a := &AsyncNotifier{Client: NewClient(time.Minute, false), Key: newSigningKey(t)}
	a.NewNotification(context.Background(), strings.TrimPrefix(p.srv.URL, "http://"), unsharedNotification(t))
	wait(t, p.entered, "discovery")

	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	// Close joined the worker, so its POST can no longer happen.
	close(p.release)
	if p.posts.Load() != 0 {
		t.Fatalf("%d posts after Close", p.posts.Load())
	}
	if res := a.NewNotification(context.Background(), strings.TrimPrefix(p.srv.URL, "http://"), unsharedNotification(t)); res.Outcome != SendPostFailed {
		t.Fatalf("send after Close: %+v", res)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if p.discovers.Load() != 1 || p.posts.Load() != 0 {
		t.Fatalf("discovers %d posts %d", p.discovers.Load(), p.posts.Load())
	}
}

func TestAsyncNotifierCloseReleasesStripeWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var a AsyncNotifier
		host := "peer.example"
		held, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
		defer func() {
			close(release)
			_ = a.Close()
			if a.inFlight != 0 {
				t.Errorf("active after join = %d", a.inFlight)
			}
		}()
		send := func(check func(context.Context) bool) {
			t.Helper()
			n, err := (ShareStateNotification{
				Type: NotificationShareChangePermission, SenderDomain: "sender.example",
				ResourceType: "file", ShareType: "user", ProviderID: "same-grant", Permissions: []string{"read"},
			}).Build()
			if err != nil {
				t.Fatal(err)
			}
			n.StillCurrent = check
			if result := a.NewNotification(t.Context(), host, n); result != (SendResult{}) {
				t.Fatalf("send admission: %+v", result)
			}
		}
		send(func(context.Context) bool {
			close(held)
			<-release
			return true
		})
		<-held

		var checked bool
		send(func(context.Context) bool {
			checked = true
			return false
		})
		synctest.Wait()
		before := a.inFlight

		go func() {
			_ = a.Close()
			close(closed)
		}()
		synctest.Wait()
		if before != 2 || a.inFlight != 1 || checked {
			t.Errorf("admitted before Close = %d, active after cancellation = %d, waiter checked = %t", before, a.inFlight, checked)
		}
		select {
		case <-closed:
			t.Error("Close returned before the holder finished")
		default:
		}
	})
}

func TestAsyncNotifierOutlivesCallerContext(t *testing.T) {
	p := newGatedPeer(t)
	close(p.release)
	c := NewClientWithConfig(ocmclient.TransportConfig{Timeout: time.Minute, AllowLoopback: true})
	a := &AsyncNotifier{Client: c, Key: newSigningKey(t)}
	ctx, cancel := context.WithCancel(context.Background())
	a.NewNotification(ctx, strings.TrimPrefix(p.srv.URL, "http://"), unsharedNotification(t))
	cancel()
	wait(t, p.posted, "notification")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if p.posts.Load() != 1 {
		t.Fatalf("posts %d", p.posts.Load())
	}
}

func TestAsyncNotifierCloseRacesAdmission(t *testing.T) {
	p := newGatedPeer(t)
	a := &AsyncNotifier{Client: NewClient(time.Minute, false), Key: newSigningKey(t)}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.NewNotification(context.Background(), strings.TrimPrefix(p.srv.URL, "http://"), unsharedNotification(t))
		}()
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	close(p.release)
	wg.Wait()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	// Admitted sends were cancelled in discovery; later ones were refused.
	if p.posts.Load() != 0 {
		t.Fatalf("%d posts after Close", p.posts.Load())
	}
}

func TestAsyncNotifierBoundsInFlightAttempts(t *testing.T) {
	p := newGatedPeer(t)
	host := strings.TrimPrefix(p.srv.URL, "http://")
	c := NewClientWithConfig(ocmclient.TransportConfig{Timeout: time.Minute, AllowLoopback: true})
	a := &AsyncNotifier{Client: c, Key: newSigningKey(t)}
	t.Cleanup(func() { _ = a.Close() })
	for i := 0; i < maxInFlightNotifications; i++ {
		if res := a.NewNotification(context.Background(), host, unsharedNotification(t)); res != (SendResult{}) {
			t.Fatalf("admission %d: %+v", i, res)
		}
	}
	for i := 0; i < maxInFlightNotifications; i++ {
		wait(t, p.entered, "discovery")
	}
	res := a.NewNotification(context.Background(), host, unsharedNotification(t))
	if res.Outcome != SendPostFailed || res.Failure != FailureSaturated {
		t.Fatalf("send over the limit: %+v", res)
	}

	close(p.release)
	for i := 0; i < maxInFlightNotifications; i++ {
		wait(t, p.posted, "notification")
	}
	// The refused notice made no request; finished attempts free their slots.
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.mu.Lock()
		n := a.inFlight
		a.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d attempts still in flight", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if res := a.NewNotification(context.Background(), host, unsharedNotification(t)); res != (SendResult{}) {
		t.Fatalf("admission after the batch: %+v", res)
	}
	wait(t, p.posted, "notification")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if d, n := p.discovers.Load(), p.posts.Load(); d != maxInFlightNotifications+1 || n != maxInFlightNotifications+1 {
		t.Fatalf("discovers %d posts %d", d, n)
	}
}

type blockedNotificationLog struct {
	entered chan struct{}
	release chan struct{}
}

func (w blockedNotificationLog) Write(b []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(b), nil
}

func TestAsyncNotifierRefusalsDoNotWaitForLogger(t *testing.T) {
	p := newGatedPeer(t)
	host := strings.TrimPrefix(p.srv.URL, "http://")
	a := &AsyncNotifier{Client: NewClient(time.Minute, false), Key: newSigningKey(t)}
	t.Cleanup(func() { _ = a.Close() })
	n := unsharedNotification(t)
	for i := 0; i < maxInFlightNotifications; i++ {
		a.NewNotification(context.Background(), host, n)
	}
	for i := 0; i < maxInFlightNotifications; i++ {
		wait(t, p.entered, "discovery")
	}

	sink := blockedNotificationLog{entered: make(chan struct{}, 1), release: make(chan struct{})}
	defer close(sink.release)
	log := zerolog.New(sink)
	ctx := appctx.WithLogger(context.Background(), &log)
	refuse := func(want SendFailure) {
		t.Helper()
		result := make(chan SendResult, 1)
		go func() { result <- a.NewNotification(ctx, host, n) }()
		select {
		case got := <-result:
			if got.Outcome != SendPostFailed || got.Failure != want {
				t.Fatalf("refusal %+v, want %s", got, want)
			}
		case <-sink.entered:
			t.Fatalf("%s refusal wrote synchronously to the blocked logger", want)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s refusal did not return", want)
		}
	}
	before := testutil.ToFloat64(saturatedNotifications)
	refuse(FailureSaturated)
	if got := testutil.ToFloat64(saturatedNotifications); got != before+1 {
		t.Fatalf("saturation count %v, want %v", got, before+1)
	}

	done := make(chan struct{})
	go func() {
		_ = a.Close()
		close(done)
	}()
	wait(t, done, "Close with the logger blocked")
	refuse(FailureCanceled)
	if p.discovers.Load() != maxInFlightNotifications || p.posts.Load() != 0 {
		t.Fatalf("discovers %d, posts %d", p.discovers.Load(), p.posts.Load())
	}
}
