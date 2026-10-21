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
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/rs/zerolog"
)

const senderSecret = "outgoing-token-9c1e"

type notificationPeer struct {
	srv           *httptest.Server
	mu            sync.Mutex
	posts         int
	lastBody      []byte
	lastType      string
	caps          []string
	enabled       bool
	wellKnownOff  bool
	legacyOff     bool
	hugeDiscovery bool
	status        int
	header        http.Header
	respBody      []byte
	hangup        bool
	redirectHits  int
	legacyHits    int
	endpoint      string
}

func newNotificationPeer(t *testing.T) *notificationPeer {
	p := &notificationPeer{caps: []string{"invites", "notifications"}, enabled: true, status: http.StatusCreated}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *notificationPeer) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch r.URL.Path {
	case "/.well-known/ocm", "/ocm-provider":
		if r.URL.Path == "/ocm-provider" {
			p.legacyHits++
		}
		if (r.URL.Path == "/.well-known/ocm" && p.wellKnownOff) || (r.URL.Path == "/ocm-provider" && p.legacyOff) {
			http.NotFound(w, r)
			return
		}
		if p.hugeDiscovery {
			_, _ = w.Write(bytes.Repeat([]byte(" "), int(DefaultResponseLimit)+10))
			return
		}
		_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{
			Enabled:      p.enabled,
			APIVersion:   "1.4.0",
			Endpoint:     p.discoveredEndpoint(),
			Capabilities: p.caps,
		})
	case "/elsewhere":
		p.redirectHits++
		w.WriteHeader(http.StatusCreated)
	case "/ocm/notifications":
		p.posts++
		p.lastBody, _ = io.ReadAll(r.Body)
		p.lastType = r.Header.Get("Content-Type")
		if p.hangup {
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			return
		}
		for k, v := range p.header {
			w.Header()[k] = v
		}
		w.WriteHeader(p.status)
		_, _ = w.Write(p.respBody)
	default:
		http.NotFound(w, r)
	}
}

func (p *notificationPeer) discoveredEndpoint() string {
	if p.endpoint != "" {
		return p.endpoint
	}
	return p.srv.URL + "/ocm"
}

func (p *notificationPeer) postCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.posts
}

func unsharedNotification(t *testing.T) *OutgoingNotification {
	t.Helper()
	n, err := ShareStateNotification{
		Type:         NotificationShareUnshared,
		SenderDomain: "cernbox.example.org",
		ResourceType: "file",
		ShareType:    "user",
		ProviderID:   "42",
		SharedSecret: senderSecret,
	}.Build()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func sendWithLogs(t *testing.T, peer *notificationPeer, n *OutgoingNotification) (SendResult, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := appctx.WithLogger(context.Background(), &logger)
	res := NewClient(5*time.Second, false).NewNotification(ctx, peer.srv.URL, n)
	return res, logs.String()
}

func TestNewNotificationTransportTable(t *testing.T) {
	retry := http.Header{}
	retry.Set("Retry-After", "60")
	cases := []struct {
		name    string
		setup   func(p *notificationPeer)
		outcome SendOutcome
		class   NotificationStatusClass
		posts   int
		retry   time.Duration
	}{
		{name: "capability missing", setup: func(p *notificationPeer) { p.caps = []string{"invites", "exchange-token"} }, outcome: SendCapabilityMissing},
		{name: "discovery disabled", setup: func(p *notificationPeer) { p.enabled = false }, outcome: SendCapabilityMissing},
		{name: "no discovery", setup: func(p *notificationPeer) { p.wellKnownOff, p.legacyOff = true, true }, outcome: SendDiscoveryFailed},
		{name: "oversized discovery", setup: func(p *notificationPeer) { p.hugeDiscovery = true }, outcome: SendDiscoveryFailed},
		{name: "no legacy ocm-provider fallback", setup: func(p *notificationPeer) { p.wellKnownOff = true }, outcome: SendDiscoveryFailed},
		{name: "201 bodyless", outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "200 with ignored body", setup: func(p *notificationPeer) { p.status, p.respBody = 200, []byte(`{"x":1}`) }, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "204", setup: func(p *notificationPeer) { p.status = 204 }, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "oversized 201 body is bounded and ignored", setup: func(p *notificationPeer) {
			p.respBody = bytes.Repeat([]byte("a"), int(DefaultResponseLimit)*2)
		}, outcome: SendDelivered, class: NotificationStatusCreated, posts: 1},
		{name: "400", setup: func(p *notificationPeer) { p.status = 400 }, outcome: SendRejected, class: NotificationStatusBadRequest, posts: 1},
		{name: "401 is not retried", setup: func(p *notificationPeer) { p.status = 401 }, outcome: SendRejected, class: NotificationStatusUnauthorized, posts: 1},
		{name: "403", setup: func(p *notificationPeer) { p.status = 403 }, outcome: SendRejected, class: NotificationStatusForbidden, posts: 1},
		{name: "404", setup: func(p *notificationPeer) { p.status = 404 }, outcome: SendRejected, class: NotificationStatusNotFound, posts: 1},
		{name: "501", setup: func(p *notificationPeer) { p.status = 501 }, outcome: SendRejected, class: NotificationStatusNotImplemented, posts: 1},
		{name: "503 with Retry-After is not retried", setup: func(p *notificationPeer) { p.status, p.header = 503, retry }, outcome: SendRejected, class: NotificationStatusUnavailable, posts: 1, retry: time.Minute},
		{name: "503 reflecting the secret", setup: func(p *notificationPeer) {
			p.status, p.respBody = 503, []byte(senderSecret)
			p.header = http.Header{"Retry-After": []string{senderSecret}, "X-Echo": []string{senderSecret}}
		}, outcome: SendRejected, class: NotificationStatusUnavailable, posts: 1},
		{name: "500", setup: func(p *notificationPeer) { p.status = 500 }, outcome: SendRejected, posts: 1},
		{name: "307 redirect is not followed", setup: func(p *notificationPeer) {
			p.status, p.header = 307, http.Header{"Location": []string{"/elsewhere"}}
		}, outcome: SendRejected, posts: 1},
		{name: "308 redirect is not followed", setup: func(p *notificationPeer) {
			p.status, p.header = 308, http.Header{"Location": []string{"/elsewhere"}}
		}, outcome: SendRejected, posts: 1},
		{name: "connection dropped", setup: func(p *notificationPeer) { p.hangup = true }, outcome: SendPostFailed, posts: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer := newNotificationPeer(t)
			if tc.setup != nil {
				tc.setup(peer)
			}
			res, logs := sendWithLogs(t, peer, unsharedNotification(t))
			if res.Outcome != tc.outcome || res.Class != tc.class || res.RetryAfter != tc.retry {
				t.Fatalf("result %+v", res)
			}
			if got := peer.postCount(); got != tc.posts {
				t.Fatalf("posts %d, want %d", got, tc.posts)
			}
			if peer.redirectHits != 0 || peer.legacyHits != 0 {
				t.Fatalf("redirect followed %d times, legacy discovery %d times", peer.redirectHits, peer.legacyHits)
			}
			if strings.Contains(logs, senderSecret) || strings.Contains(fmt.Sprintf("%+v", res), senderSecret) {
				t.Fatalf("secret in logs or result: %s %+v", logs, res)
			}
			if !strings.Contains(logs, `"outcome":"`+string(tc.outcome)+`"`) {
				t.Fatalf("outcome not logged: %s", logs)
			}
		})
	}
}

// Our own receiver parses the share-state payload as a spec-shaped notification.
func TestShareStatePayloadFixtures(t *testing.T) {
	cases := []struct {
		name   string
		in     ShareStateNotification
		secret string
		perms  []string
	}{
		{
			name:   "unshared carries the outgoing token",
			in:     ShareStateNotification{Type: NotificationShareUnshared, SenderDomain: "provider.example.org", ResourceType: "file", ShareType: "user", ProviderID: "42", SharedSecret: senderSecret},
			secret: senderSecret,
		},
		{
			name:   "accepted carries the received secret and the provider's id",
			in:     ShareStateNotification{Type: NotificationShareAccepted, SenderDomain: "recipient.example.org", ResourceType: "file", ShareType: "user", ProviderID: "remote-7", SharedSecret: senderSecret},
			secret: senderSecret,
		},
		{
			name: "declined without a secret",
			in:   ShareStateNotification{Type: NotificationShareDeclined, SenderDomain: "recipient.example.org", ResourceType: "folder", ProviderID: "remote-7"},
		},
		{
			name:  "change permission carries permissions only",
			in:    ShareStateNotification{Type: NotificationShareChangePermission, SenderDomain: "provider.example.org", ResourceType: "file", ShareType: "user", ProviderID: "42", Permissions: []string{"read"}},
			perms: []string{"read"},
		},
	}
	reg := NewNotificationRegistry()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer := newNotificationPeer(t)
			n, err := tc.in.Build()
			if err != nil {
				t.Fatal(err)
			}
			res, _ := sendWithLogs(t, peer, n)
			if res.Outcome != SendDelivered {
				t.Fatalf("%+v", res)
			}
			if peer.lastType != "application/json" {
				t.Fatalf("content type %q", peer.lastType)
			}
			var wire map[string]any
			if err := json.Unmarshal(peer.lastBody, &wire); err != nil {
				t.Fatal(err)
			}
			if wire["notificationType"] != tc.in.Type || wire["senderDomain"] != tc.in.SenderDomain || wire["resourceType"] != tc.in.ResourceType {
				t.Fatalf("wire %v", wire)
			}
			body := wire["notification"].(map[string]any)
			file := body["file"].(map[string]any)
			if file["providerId"] != tc.in.ProviderID {
				t.Fatalf("file %v", file)
			}
			if _, has := wire["providerId"]; has {
				t.Fatalf("deprecated top-level providerId sent: %v", wire)
			}
			// Draft-silent optional secret extension.
			got, has := body["sharedSecret"]
			if (tc.secret != "") != has || (has && got != tc.secret) {
				t.Fatalf("sharedSecret present=%v", has)
			}
			if p, ok := file["permissions"]; (len(tc.perms) > 0) != ok || (ok && fmt.Sprint(p) != fmt.Sprint([]any{"read"})) {
				t.Fatalf("permissions %v", p)
			}
			parsed, err := reg.Parse(peer.lastBody)
			if err != nil || parsed.Class != NotificationClassSpec {
				t.Fatalf("receiver parse %v class %s", err, parsed.Class)
			}
		})
	}
}

func TestShareStateBuildRejectsInvalidPayloads(t *testing.T) {
	bad := []ShareStateNotification{
		{Type: NotificationUserRemoved, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1"},
		{Type: NotificationRequestShare, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1"},
		{Type: NotificationShareUnshared, ResourceType: "file", ProviderID: "1"},
		{Type: NotificationShareUnshared, SenderDomain: "a.example.org", ProviderID: "1"},
		{Type: NotificationShareUnshared, SenderDomain: "a.example.org", ResourceType: "file"},
		{Type: NotificationShareUnshared, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1", Permissions: []string{"read"}},
		{Type: NotificationShareChangePermission, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1", SharedSecret: "s"},
		{Type: NotificationShareUnshared, SenderDomain: "a.example.org:443", ResourceType: "file", ProviderID: "1"},
		{Type: NotificationShareUnshared, SenderDomain: "192.0.2.1", ResourceType: "file", ProviderID: "1"},
		{Type: NotificationShareUnshared, SenderDomain: "https://a.example.org", ResourceType: "file", ProviderID: "1"},
	}
	good := ShareStateNotification{Type: NotificationShareUnshared, SenderDomain: "a.example.org", ResourceType: "file", ProviderID: "1"}
	if _, err := good.Build(); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	for i, s := range bad {
		if _, err := s.Build(); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestOutgoingNotificationFormsRedactSecret(t *testing.T) {
	n := unsharedNotification(t)
	raw, _ := json.Marshal(n)
	out := fmt.Sprintf("%s %v %+v %#v %v", raw, n, *n, n, n.SharedSecret)
	if strings.Contains(out, senderSecret) {
		t.Fatalf("secret leaked: %s", out)
	}
	wire, err := n.wireBody()
	if err != nil || !strings.Contains(string(wire), senderSecret) {
		t.Fatal("wire body lost the secret")
	}
}

func TestNewNotificationNilAndUnreachable(t *testing.T) {
	c := NewClient(time.Second, false)
	if res := c.NewNotification(context.Background(), "127.0.0.1:1", nil); res.Outcome != SendPostFailed {
		t.Fatalf("nil: %+v", res)
	}
	if res := c.NewNotification(context.Background(), "http://127.0.0.1:1", unsharedNotification(t)); res.Outcome != SendDiscoveryFailed {
		t.Fatalf("unreachable: %+v", res)
	}
}

// rawPeer answers every request on a real TCP connection with fixed bytes,
// which can be a status line or header that net/http fails to parse.
type rawPeer struct {
	ln       net.Listener
	mu       sync.Mutex
	requests []string
	response string
	hang     bool
	got      chan struct{}
}

func newRawPeer(t *testing.T, response string) *rawPeer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &rawPeer{ln: ln, response: response, got: make(chan struct{}, 8)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.handle(conn)
		}
	}()
	return p
}

func (p *rawPeer) handle(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, req.Body)
	p.mu.Lock()
	p.requests = append(p.requests, req.Method+" "+req.URL.Path)
	p.mu.Unlock()
	p.got <- struct{}{}
	if p.hang {
		// Wait for the client to give up.
		_, _ = io.Copy(io.Discard, br)
		return
	}
	_, _ = conn.Write([]byte(p.response))
}

func (p *rawPeer) url() string { return "http://" + p.ln.Addr().String() }

func (p *rawPeer) count(request string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.requests {
		if r == request {
			n++
		}
	}
	return n
}

func assertNoCanary(t *testing.T, res SendResult, logs string) {
	t.Helper()
	out := fmt.Sprintf("%v %+v %#v %s", res, res, res, logs)
	if strings.Contains(out, senderSecret) {
		t.Fatalf("secret leaked: %s", out)
	}
}

var malformedResponses = map[string]string{
	"status line":    "HTTP/1.1 " + senderSecret + "\r\n\r\n",
	"status code":    "HTTP/1.1 2" + senderSecret + " Created\r\n\r\n",
	"version":        "HTTP/" + senderSecret + " 201 Created\r\n\r\n",
	"header line":    "HTTP/1.1 201 Created\r\n" + senderSecret + "\r\n\r\n",
	"content length": "HTTP/1.1 201 Created\r\nContent-Length: " + senderSecret + "\r\n\r\n",
}

// A malformed status line or header makes net/http return an error that
// quotes the peer bytes; it must not reach the result or the logs.
func TestNewNotificationMalformedPostResponseOverTCP(t *testing.T) {
	for name, response := range malformedResponses {
		t.Run(name, func(t *testing.T) {
			raw := newRawPeer(t, response)
			peer := newNotificationPeer(t)
			peer.endpoint = raw.url() + "/ocm"
			res, logs := sendWithLogs(t, peer, unsharedNotification(t))
			if res.Outcome != SendPostFailed || res.Failure != FailureError {
				t.Fatalf("result %+v", res)
			}
			if got := raw.count("POST /ocm/notifications"); got != 1 {
				t.Fatalf("posts %d", got)
			}
			assertNoCanary(t, res, logs)
		})
	}
}

func TestNewNotificationMalformedDiscoveryOverTCP(t *testing.T) {
	for name, response := range malformedResponses {
		t.Run(name, func(t *testing.T) {
			raw := newRawPeer(t, response)
			var logs bytes.Buffer
			logger := zerolog.New(&logs).Level(zerolog.DebugLevel)
			ctx := appctx.WithLogger(context.Background(), &logger)
			res := NewClient(5*time.Second, false).NewNotification(ctx, raw.url(), unsharedNotification(t))
			if res.Outcome != SendDiscoveryFailed || res.Failure != FailureError {
				t.Fatalf("result %+v", res)
			}
			if raw.count("GET /ocm-provider") != 0 || raw.count("POST /ocm/notifications") != 0 {
				t.Fatalf("requests %v", raw.requests)
			}
			assertNoCanary(t, res, logs.String())
		})
	}
}

func TestNewNotificationWellFormedResponsesOverTCP(t *testing.T) {
	cases := []struct {
		response string
		outcome  SendOutcome
		retry    time.Duration
	}{
		{"HTTP/1.1 201 Created\r\nContent-Length: 0\r\n\r\n", SendDelivered, 0},
		{"HTTP/1.1 503 Service Unavailable\r\nRetry-After: 30\r\nContent-Length: 0\r\n\r\n", SendRejected, 30 * time.Second},
		{"HTTP/1.1 503 Service Unavailable\r\nRetry-After: " + senderSecret + "\r\nContent-Length: 0\r\n\r\n", SendRejected, 0},
	}
	for _, tc := range cases {
		raw := newRawPeer(t, tc.response)
		peer := newNotificationPeer(t)
		peer.endpoint = raw.url() + "/ocm"
		res, logs := sendWithLogs(t, peer, unsharedNotification(t))
		if res.Outcome != tc.outcome || res.RetryAfter != tc.retry || res.Failure != "" {
			t.Fatalf("%q: %+v", tc.response, res)
		}
		if raw.count("POST /ocm/notifications") != 1 {
			t.Fatalf("posts %v", raw.requests)
		}
		assertNoCanary(t, res, logs)
	}
}

func TestNewNotificationTimeoutAndCancel(t *testing.T) {
	raw := newRawPeer(t, "")
	raw.hang = true
	peer := newNotificationPeer(t)
	peer.endpoint = raw.url() + "/ocm"
	res := NewClient(300*time.Millisecond, false).NewNotification(context.Background(), peer.srv.URL, unsharedNotification(t))
	if res.Outcome != SendPostFailed || res.Failure != FailureTimeout {
		t.Fatalf("timeout: %+v", res)
	}
	<-raw.got

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-raw.got
		cancel()
	}()
	res = NewClient(5*time.Second, false).NewNotification(ctx, peer.srv.URL, unsharedNotification(t))
	if res.Outcome != SendPostFailed || res.Failure != FailureCanceled {
		t.Fatalf("cancel: %+v", res)
	}
	if raw.count("POST /ocm/notifications") != 2 {
		t.Fatalf("posts %v", raw.requests)
	}
}

func TestParseRetryAfter(t *testing.T) {
	for v, want := range map[string]time.Duration{
		"0": 0, "30": 30 * time.Second, "999999999": 999999999 * time.Second,
		"": 0, "1000000000": 0, "-1": 0, "+5": 0, "1.5": 0, " 30": 0, "30 ": 0,
		"Wed, 21 Oct 2026 07:28:00 GMT": 0, senderSecret: 0,
	} {
		if got := parseRetryAfter(v); got != want {
			t.Errorf("%q: %s, want %s", v, got, want)
		}
	}
}

func TestSafePostErrorKeepsNoCause(t *testing.T) {
	cause := errors.New(senderSecret)
	for _, err := range []error{
		safePostError(context.Background(), cause),
		safePostError(context.Background(), &net.OpError{Op: "read", Err: cause}),
	} {
		if strings.Contains(fmt.Sprintf("%v %+v", err, err), senderSecret) || errors.Is(err, cause) || errors.Unwrap(err) != nil {
			t.Fatalf("cause kept: %v", err)
		}
	}
}

// The transport itself returns no error that quotes the peer bytes.
func TestPostNotificationErrorQuotesNoPeerBytes(t *testing.T) {
	for name, response := range malformedResponses {
		t.Run(name, func(t *testing.T) {
			raw := newRawPeer(t, response)
			tr := ocmNotificationTransport{c: NewClient(5*time.Second, false)}
			resp, err := tr.PostNotification(context.Background(), raw.url()+"/ocm/notifications", []byte(`{}`))
			if err == nil || resp != (NotificationResponse{}) {
				t.Fatalf("response %+v error %v", resp, err)
			}
			if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), senderSecret) || errors.Unwrap(err) != nil {
				t.Fatalf("cause kept: %v", err)
			}
		})
	}
}
