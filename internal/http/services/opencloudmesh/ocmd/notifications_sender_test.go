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
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	ocmclient "github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/cs3org/reva/v3/pkg/ocm/httpsig"
	"github.com/rs/zerolog"
)

const senderCanary = "private-payload-9c1e"

var nopLogger = zerolog.Nop()

type notificationServer struct {
	srv           *httptest.Server
	mu            sync.Mutex
	posts         int
	lastBody      []byte
	lastType      string
	caps          []string
	enabled       bool
	wellKnownOff  bool
	hugeDiscovery bool
	status        int
	header        http.Header
	respBody      []byte
	hangup        bool
	redirectHits  int
	legacyHits    int
	endpoint      string
}

func newNotificationServer(t *testing.T) *notificationServer {
	p := &notificationServer{caps: []string{"invites", "notifications"}, enabled: true, status: http.StatusCreated}
	p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *notificationServer) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch r.URL.Path {
	case "/.well-known/ocm", "/ocm-provider":
		if r.URL.Path == "/ocm-provider" {
			p.legacyHits++
		}
		if r.URL.Path == "/.well-known/ocm" && p.wellKnownOff {
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

func (p *notificationServer) discoveredEndpoint() string {
	if p.endpoint != "" {
		return p.endpoint
	}
	return p.srv.URL + "/ocm"
}

func (p *notificationServer) host() string { return strings.TrimPrefix(p.srv.URL, "http://") }

func (p *notificationServer) postCount() int {
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
	}.Build()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func sendWithLogs(t *testing.T, peer *notificationServer, n *OutgoingNotification) (SendResult, string) {
	t.Helper()
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := appctx.WithLogger(context.Background(), &logger)
	res := NewClient(5*time.Second, false).NewNotification(ctx, peer.host(), n, nil)
	return res, logs.String()
}

func TestNewNotificationTransportTable(t *testing.T) {
	retry := http.Header{}
	retry.Set("Retry-After", "60")
	cases := []struct {
		name    string
		setup   func(p *notificationServer)
		outcome SendOutcome
		class   int
		posts   int
		retry   time.Duration
	}{
		{name: "capability missing", setup: func(p *notificationServer) { p.caps = []string{"invites", "exchange-token"} }, outcome: SendCapabilityMissing},
		{name: "discovery disabled", setup: func(p *notificationServer) { p.enabled = false }, outcome: SendCapabilityMissing},
		{name: "oversized discovery", setup: func(p *notificationServer) { p.hugeDiscovery = true }, outcome: SendDiscoveryFailed},
		{name: "no legacy ocm-provider fallback", setup: func(p *notificationServer) { p.wellKnownOff = true }, outcome: SendDiscoveryFailed},
		{name: "201 bodyless", outcome: SendDelivered, class: http.StatusCreated, posts: 1},
		{name: "200 with ignored body", setup: func(p *notificationServer) { p.status, p.respBody = 200, []byte(`{"x":1}`) }, outcome: SendDelivered, class: http.StatusCreated, posts: 1},
		{name: "oversized 201 body is bounded and ignored", setup: func(p *notificationServer) {
			p.respBody = bytes.Repeat([]byte("a"), int(DefaultResponseLimit)*2)
		}, outcome: SendDelivered, class: http.StatusCreated, posts: 1},
		{name: "401 is not retried", setup: func(p *notificationServer) { p.status = 401 }, outcome: SendRejected, class: http.StatusUnauthorized, posts: 1},
		{name: "503 with Retry-After is not retried", setup: func(p *notificationServer) { p.status, p.header = 503, retry }, outcome: SendRejected, class: http.StatusServiceUnavailable, posts: 1, retry: time.Minute},
		{name: "503 reflecting private data", setup: func(p *notificationServer) {
			p.status, p.respBody = 503, []byte(senderCanary)
			p.header = http.Header{"Retry-After": []string{senderCanary}, "X-Echo": []string{senderCanary}}
		}, outcome: SendRejected, class: http.StatusServiceUnavailable, posts: 1},
		{name: "307 redirect is not followed", setup: func(p *notificationServer) {
			p.status, p.header = 307, http.Header{"Location": []string{"/elsewhere"}}
		}, outcome: SendRejected, posts: 1},
		{name: "308 redirect is not followed", setup: func(p *notificationServer) {
			p.status, p.header = 308, http.Header{"Location": []string{"/elsewhere"}}
		}, outcome: SendRejected, posts: 1},
		{name: "connection dropped", setup: func(p *notificationServer) { p.hangup = true }, outcome: SendPostFailed, posts: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer := newNotificationServer(t)
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
			if strings.Contains(logs, senderCanary) || strings.Contains(fmt.Sprintf("%+v", res), senderCanary) {
				t.Fatalf("private data in logs or result: %s %+v", logs, res)
			}
			if !strings.Contains(logs, `"outcome":"`+string(tc.outcome)+`"`) {
				t.Fatalf("outcome not logged: %s", logs)
			}
		})
	}
}

func TestShareStatePayloadFixtures(t *testing.T) {
	cases := []struct {
		name    string
		in      ShareStateNotification
		message string
		want    string
	}{
		{
			name: "unshared preserves provider grant",
			in:   ShareStateNotification{Type: NotificationShareUnshared, SenderDomain: "Provider.Example.org:0443", ResourceType: "file", ShareType: "user", ProviderID: " grant@provider %2F "},
			want: `{"notificationType":"SHARE_UNSHARED","senderDomain":"provider.example.org:0443","resourceType":"file","shareType":"user","notification":{"file":{"providerId":" grant@provider %2F "}}}`,
		},
		{
			name:    "accepted preserves remote grant and message",
			in:      ShareStateNotification{Type: NotificationShareAccepted, SenderDomain: "Recipient.Example.org:8443", ResourceType: "file", ShareType: "user", ProviderID: "remote@id %25"},
			message: "accepted privately",
			want:    `{"notificationType":"SHARE_ACCEPTED","senderDomain":"recipient.example.org:8443","resourceType":"file","shareType":"user","notification":{"message":"accepted privately","file":{"providerId":"remote@id %25"}}}`,
		},
		{
			name: "declined folder without share type",
			in:   ShareStateNotification{Type: NotificationShareDeclined, SenderDomain: "[2001:DB8::1]:8443", ResourceType: "folder", ProviderID: "remote-7"},
			want: `{"notificationType":"SHARE_DECLINED","senderDomain":"[2001:db8::1]:8443","resourceType":"folder","notification":{"file":{"providerId":"remote-7"}}}`,
		},
		{
			name: "changed permissions preserve order and duplicates",
			in:   ShareStateNotification{Type: NotificationShareChangePermission, SenderDomain: "provider.example.org", ResourceType: "file", ShareType: "user", ProviderID: "42", Permissions: []string{"write", "read", "read"}},
			want: `{"notificationType":"SHARE_CHANGE_PERMISSION","senderDomain":"provider.example.org","resourceType":"file","shareType":"user","notification":{"file":{"providerId":"42","permissions":["write","read","read"]}}}`,
		},
	}
	reg := NewNotificationRegistry()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			peer := newNotificationServer(t)
			n, err := tc.in.Build()
			if err != nil {
				t.Fatal(err)
			}
			n.Message = tc.message
			res, _ := sendWithLogs(t, peer, n)
			if res.Outcome != SendDelivered {
				t.Fatalf("%+v", res)
			}
			if peer.lastType != "application/json" {
				t.Fatalf("content type %q", peer.lastType)
			}
			var got, want any
			if err := json.Unmarshal(peer.lastBody, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("wire %s, want %s", peer.lastBody, tc.want)
			}
			parsed, err := reg.Parse(peer.lastBody)
			if err != nil || parsed.Class != NotificationClassSpec || parsed.ProviderID != tc.in.ProviderID {
				t.Fatalf("receiver parse %v: %+v", err, parsed)
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

func TestOutgoingNotificationFormsOmitPayload(t *testing.T) {
	n := unsharedNotification(t)
	n.Message = senderCanary
	n.File.ProviderID = senderCanary
	raw, _ := json.Marshal(n)
	out := fmt.Sprintf("%s %v %+v %#v", raw, n, *n, n)
	if strings.Contains(out, senderCanary) {
		t.Fatalf("payload leaked: %s", out)
	}
	wire, err := n.wireBody()
	if err != nil || !strings.Contains(string(wire), senderCanary) {
		t.Fatal("wire body lost payload values")
	}
}

func TestNewNotificationNilAndUnreachable(t *testing.T) {
	c := NewClient(time.Second, false)
	if res := c.NewNotification(context.Background(), "127.0.0.1:1", nil, nil); res.Outcome != SendPostFailed {
		t.Fatalf("nil: %+v", res)
	}
	if res := c.NewNotification(context.Background(), "127.0.0.1:1", unsharedNotification(t), nil); res.Outcome != SendDiscoveryFailed {
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
	t.Cleanup(func() { _ = ln.Close() })
	p := &rawPeer{ln: ln, response: response, got: make(chan struct{}, 8)}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
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
	if strings.Contains(out, senderCanary) {
		t.Fatalf("payload leaked: %s", out)
	}
}

var malformedResponses = map[string]string{
	"status line":    "HTTP/1.1 " + senderCanary + "\r\n\r\n",
	"status code":    "HTTP/1.1 2" + senderCanary + " Created\r\n\r\n",
	"version":        "HTTP/" + senderCanary + " 201 Created\r\n\r\n",
	"header line":    "HTTP/1.1 201 Created\r\n" + senderCanary + "\r\n\r\n",
	"content length": "HTTP/1.1 201 Created\r\nContent-Length: " + senderCanary + "\r\n\r\n",
}

// A malformed status line or header makes net/http return an error that
// quotes the peer bytes; it must not reach the result or the logs.
func TestNewNotificationMalformedPostResponseOverTCP(t *testing.T) {
	for name, response := range malformedResponses {
		t.Run(name, func(t *testing.T) {
			raw := newRawPeer(t, response)
			peer := newNotificationServer(t)
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
			res := NewClient(5*time.Second, false).NewNotification(ctx, raw.ln.Addr().String(), unsharedNotification(t), nil)
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

func TestNewNotificationTimeoutAndCancel(t *testing.T) {
	raw := newRawPeer(t, "")
	raw.hang = true
	peer := newNotificationServer(t)
	peer.endpoint = raw.url() + "/ocm"
	res := NewClient(300*time.Millisecond, false).NewNotification(context.Background(), peer.host(), unsharedNotification(t), nil)
	if res.Outcome != SendPostFailed || res.Failure != FailureTimeout {
		t.Fatalf("timeout: %+v", res)
	}
	select {
	case <-raw.got:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout POST not observed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		select {
		case <-raw.got:
			cancel()
		case <-ctx.Done():
		}
	}()
	res = NewClient(5*time.Second, false).NewNotification(ctx, peer.host(), unsharedNotification(t), nil)
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
		"Wed, 21 Oct 2026 07:28:00 GMT": 0, senderCanary: 0,
	} {
		if got := parseRetryAfter(v); got != want {
			t.Errorf("%q: %s, want %s", v, got, want)
		}
	}
}

func TestSafePostErrorKeepsNoCause(t *testing.T) {
	cause := errors.New(senderCanary)
	for _, err := range []error{
		safePostError(context.Background(), cause),
		safePostError(context.Background(), &net.OpError{Op: "read", Err: cause}),
	} {
		if strings.Contains(fmt.Sprintf("%v %+v", err, err), senderCanary) || errors.Is(err, cause) || errors.Unwrap(err) != nil {
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
			resp, err := tr.PostNotification(context.Background(), &notificationPeer{Endpoint: raw.url() + "/ocm/notifications"}, []byte(`{}`))
			if err == nil || resp != (notificationResponse{}) {
				t.Fatalf("response %+v error %v", resp, err)
			}
			if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), senderCanary) || errors.Unwrap(err) != nil {
				t.Fatalf("cause kept: %v", err)
			}
		})
	}
}

// The sender keeps its complete host and port, lowercased only.
// Ports and IP literals are valid.
func TestShareStateBuildCanonicalSender(t *testing.T) {
	for in, want := range map[string]string{
		"a.example.org":      "a.example.org",
		"A.Example.ORG:8443": "a.example.org:8443",
		"a.example.org:0443": "a.example.org:0443",
		"192.0.2.1":          "192.0.2.1",
		"[2001:DB8::1]:8443": "[2001:db8::1]:8443",
	} {
		n, err := ShareStateNotification{Type: NotificationShareUnshared, SenderDomain: in, ResourceType: "file", ProviderID: "1"}.Build()
		if err != nil || n.SenderDomain != want {
			t.Fatalf("%q: %v %v", in, n, err)
		}
	}
	for _, in := range []string{"", "https://a.example.org", "x@a.example.org", "a.example.org:bad", "a.example.org/x", " a.example.org"} {
		if _, err := (ShareStateNotification{Type: NotificationShareUnshared, SenderDomain: in, ResourceType: "file", ProviderID: "1"}).Build(); err == nil {
			t.Fatalf("%q accepted", in)
		}
	}
}

type countingTransport struct{ n atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, errors.New("no request expected")
}

// Valid identities that HTTP cannot reach as written, and invalid ones, end
// without any request: the identity is never rewritten to reach it.
func TestNotificationToUnusableHostMakesNoRequest(t *testing.T) {
	for _, domain := range []string{"a.example.org:", "a.example.org:999999999999999999999999", "a.example.org:0", "[v1.a]",
		"a%2eexample.org", "https://a.example.org", "x@a.example.org", "a.example.org/path", "a.example.org:bad"} {
		c := NewClient(time.Second, false)
		rt := &countingTransport{}
		c.client.Transport = rt
		res := c.NewNotification(context.Background(), domain, unsharedNotification(t), nil)
		if res.Outcome != SendDiscoveryFailed || res.Failure != FailureError || rt.n.Load() != 0 {
			t.Fatalf("%q: %+v requests %d", domain, res, rt.n.Load())
		}
	}
}

func TestNotificationSigningBehavior(t *testing.T) {
	t.Run("signing failure sends zero posts", func(t *testing.T) {
		var posts atomic.Int32
		peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(peer.Close)
		transport := ocmNotificationTransport{c: NewClient(5*time.Second, false), key: &httpsig.Key{}}
		_, err := transport.PostNotification(context.Background(), &notificationPeer{Endpoint: peer.URL}, []byte(`{}`))
		if !errors.Is(err, errSigningFailed) || posts.Load() != 0 {
			t.Fatalf("signing error %v, posts %d", err, posts.Load())
		}
	})

	t.Run("advertising peer without key sends zero posts", func(t *testing.T) {
		peer := newNotificationServer(t)
		peer.caps = []string{"notifications", "http-sig"}
		ctx := appctx.WithLogger(context.Background(), &nopLogger)
		res := NewClient(5*time.Second, false).NewNotification(ctx, peer.host(), unsharedNotification(t), nil)
		if res.Outcome != SendPostFailed || res.Failure != FailureNoSigningKey || peer.postCount() != 0 {
			t.Fatalf("result %+v, posts %d", res, peer.postCount())
		}
	})

	t.Run("configured key signs the exact sent bytes", func(t *testing.T) {
		key := newSigningKey(t)
		jwks, err := key.JWKS()
		if err != nil {
			t.Fatal(err)
		}
		var gotBody []byte
		var gotReq *http.Request
		peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/.well-known/ocm":
				_ = json.NewEncoder(w).Encode(wellknown.OcmDiscoveryData{Enabled: true, Endpoint: "http://" + r.Host, Capabilities: []string{"notifications"}})
			case "/notifications":
				gotBody, _ = io.ReadAll(io.LimitReader(r.Body, DefaultResponseLimit))
				gotReq = r.Clone(r.Context())
				w.WriteHeader(http.StatusCreated)
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(peer.Close)
		ctx := appctx.WithLogger(context.Background(), &nopLogger)
		res := NewClient(5*time.Second, false).NewNotification(ctx, strings.TrimPrefix(peer.URL, "http://"), unsharedNotification(t), key)
		if res.Outcome != SendDelivered || gotReq == nil {
			t.Fatalf("result %+v, request %v", res, gotReq)
		}
		// Verify the sending service's key against the bytes received on the wire.
		if err := httpsig.Verify(gotReq, gotBody, "", func() ([]byte, error) { return jwks, nil }); err != nil {
			t.Fatalf("sent bytes do not verify against the sending key: %v", err)
		}
		want := `{"notificationType":"SHARE_UNSHARED","senderDomain":"cernbox.example.org","resourceType":"file","shareType":"user","notification":{"file":{"providerId":"42"}}}`
		if string(gotBody) != want {
			t.Fatalf("body %s, want %s", gotBody, want)
		}
	})
}

func TestNotificationGuardedTransport(t *testing.T) {
	for _, tc := range []struct {
		name          string
		allowLoopback bool
		endpoint      string
		outcome       SendOutcome
		posts         int
		wantPolicy    bool
	}{
		{name: "explicit loopback fixture exception", allowLoopback: true, outcome: SendDelivered, posts: 1},
		{name: "discovery denied by default", outcome: SendDiscoveryFailed, wantPolicy: true},
		{name: "advertised destination denied", allowLoopback: true, endpoint: "https://192.0.2.1:8443/ocm", outcome: SendPostFailed, wantPolicy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			peer := newNotificationServer(t)
			peer.endpoint = tc.endpoint
			c := NewPublicOnlyClientWithConfig(ocmclient.TransportConfig{Timeout: time.Second, AllowLoopback: tc.allowLoopback})
			base := c.Transport()
			var policy atomic.Bool
			c.client.Transport = roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				resp, err := base.RoundTrip(r)
				if errors.Is(err, ocmclient.ErrPolicyViolation) {
					policy.Store(true)
				}
				return resp, err
			})
			t.Cleanup(ocmclient.HTTPTransport(base).CloseIdleConnections)
			res := c.NewNotification(context.Background(), peer.host(), unsharedNotification(t), nil)
			if res.Outcome != tc.outcome || peer.postCount() != tc.posts || policy.Load() != tc.wantPolicy {
				t.Fatalf("result %+v, posts %d, policy refusal %v", res, peer.postCount(), policy.Load())
			}
		})
	}
}
