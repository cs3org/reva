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
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

type fakeNotificationTransport struct {
	peer        *NotificationPeer
	discoverErr error
	resp        NotificationResponse
	postErr     error
	discovers   int
	posts       int
	endpoint    string
}

func (f *fakeNotificationTransport) DiscoverNotificationPeer(context.Context, string) (*NotificationPeer, error) {
	f.discovers++
	return f.peer, f.discoverErr
}

func (f *fakeNotificationTransport) PostNotification(_ context.Context, endpoint string, _ []byte) (NotificationResponse, error) {
	f.posts++
	f.endpoint = endpoint
	return f.resp, f.postErr
}

func advertising(caps ...string) *NotificationPeer {
	return &NotificationPeer{Endpoint: "https://peer.example.org/ocm/notifications", Capabilities: caps}
}

func TestSendNotificationTransitions(t *testing.T) {
	cases := []struct {
		name    string
		tr      *fakeNotificationTransport
		outcome SendOutcome
		class   NotificationStatusClass
		failure SendFailure
		posts   int
		retry   time.Duration
	}{
		{name: "discovery failure", tr: &fakeNotificationTransport{discoverErr: errors.New("dial")}, outcome: SendDiscoveryFailed, failure: FailureError},
		{name: "nil peer", tr: &fakeNotificationTransport{}, outcome: SendCapabilityMissing},
		{name: "capability missing", tr: &fakeNotificationTransport{peer: advertising("invites", "exchange-token")}, outcome: SendCapabilityMissing},
		{name: "capability without endpoint", tr: &fakeNotificationTransport{peer: &NotificationPeer{Capabilities: []string{"notifications"}}}, outcome: SendCapabilityMissing},
		{name: "post failure", tr: &fakeNotificationTransport{peer: advertising("notifications"), postErr: errors.New("reset")}, outcome: SendPostFailed, failure: FailureError, posts: 1},
		{name: "post canceled", tr: &fakeNotificationTransport{peer: advertising("notifications"), postErr: fmt.Errorf("post: %w", context.Canceled)}, outcome: SendPostFailed, failure: FailureCanceled, posts: 1},
		{name: "post deadline", tr: &fakeNotificationTransport{peer: advertising("notifications"), postErr: fmt.Errorf("post: %w", context.DeadlineExceeded)}, outcome: SendPostFailed, failure: FailureTimeout, posts: 1},
		{name: "post net timeout", tr: &fakeNotificationTransport{peer: advertising("notifications"), postErr: &net.OpError{Op: "read", Err: timeoutError{}}}, outcome: SendPostFailed, failure: FailureTimeout, posts: 1},
		{name: "201", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 201}}, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "200 tolerated", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 200}}, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "202 tolerated", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 202}}, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "204 tolerated", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 204}}, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "400", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 400}}, outcome: SendRejected, class: NotificationStatusBadRequest, posts: 1},
		{name: "401", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 401}}, outcome: SendRejected, class: NotificationStatusUnauthorized, posts: 1},
		{name: "403", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 403}}, outcome: SendRejected, class: NotificationStatusForbidden, posts: 1},
		{name: "404", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 404}}, outcome: SendRejected, class: NotificationStatusNotFound, posts: 1},
		{name: "501", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 501}}, outcome: SendRejected, class: NotificationStatusNotImplemented, posts: 1},
		{name: "503 records Retry-After", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 503, RetryAfter: 120 * time.Second}}, outcome: SendRejected, class: NotificationStatusUnavailable, posts: 1, retry: 120 * time.Second},
		{name: "500 unclassified", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 500}}, outcome: SendRejected, posts: 1},
		{name: "302 unclassified", tr: &fakeNotificationTransport{peer: advertising("notifications"), resp: NotificationResponse{Status: 302}}, outcome: SendRejected, posts: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := SendNotification(context.Background(), tc.tr, "peer.example.org", []byte(`{}`))
			if res.Outcome != tc.outcome || res.Class != tc.class || res.RetryAfter != tc.retry || res.Failure != tc.failure {
				t.Fatalf("result %+v", res)
			}
			if tc.tr.discovers != 1 || tc.tr.posts != tc.posts {
				t.Fatalf("discovers %d posts %d, want 1 and %d", tc.tr.discovers, tc.tr.posts, tc.posts)
			}
			if tc.posts == 1 && tc.tr.endpoint != "https://peer.example.org/ocm/notifications" {
				t.Fatalf("endpoint %q", tc.tr.endpoint)
			}
		})
	}
}

type timeoutError struct{}

func (timeoutError) Error() string { return "i/o timeout" }
func (timeoutError) Timeout() bool { return true }

func TestRetryAfterIsBounded(t *testing.T) {
	cases := map[time.Duration]time.Duration{
		0:                 0,
		-time.Second:      0,
		120 * time.Second: 120 * time.Second,
		24 * time.Hour:    24 * time.Hour,
		1000 * time.Hour:  24 * time.Hour,
	}
	for in, want := range cases {
		res := ClassifyNotificationResponse(NotificationResponse{Status: 503, RetryAfter: in})
		if res.Class != NotificationStatusUnavailable || res.RetryAfter != want {
			t.Fatalf("Retry-After %s: %+v", in, res)
		}
	}
	if res := ClassifyNotificationResponse(NotificationResponse{Status: 400, RetryAfter: time.Hour}); res.RetryAfter != 0 {
		t.Fatalf("Retry-After kept outside 503: %+v", res)
	}
}

// The transport errors quote peer bytes; the result must keep only a category.
func TestSendResultKeepsNoTransportError(t *testing.T) {
	const canary = "hfiuhworzwnur98d3wjiwhr"
	leak := fmt.Errorf("wrapped: %w", &net.OpError{Op: "read", Err: errors.New(canary + "\r\nX-Leak: " + canary)})
	for _, tr := range []*fakeNotificationTransport{
		{discoverErr: leak},
		{peer: advertising("notifications"), postErr: leak},
	} {
		res := SendNotification(context.Background(), tr, "peer.example.org", []byte(`{}`))
		if res.Failure != FailureError {
			t.Fatalf("failure %+v", res)
		}
		if out := fmt.Sprintf("%v %+v %#v", res, res, res); strings.Contains(out, canary) {
			t.Fatalf("canary in %s", out)
		}
	}
}

func TestReceiverStatusNeverUnavailable(t *testing.T) {
	errs := []error{
		nil,
		&NotificationError{Code: NotificationErrMalformed},
		&NotificationError{Code: NotificationErrMissingType},
		&NotificationError{Code: NotificationErrMissingField, Field: "x"},
		&NotificationError{Code: NotificationErrInvalidField, Field: "x"},
		&NotificationError{Code: NotificationErrNotImplemented},
		errors.New("other"),
	}
	want := []NotificationStatusClass{201, 400, 400, 400, 400, 501, 400}
	for i, err := range errs {
		if got := ReceiverStatus(err); got != want[i] || got == NotificationStatusUnavailable {
			t.Fatalf("%v -> %d, want %d", err, got, want[i])
		}
	}
}
