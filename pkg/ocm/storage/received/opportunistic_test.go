package ocm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/cs3org/reva/v3/pkg/ocm/client"
	"github.com/rs/zerolog"
	"github.com/studio-b12/gowebdav"
)

const ocmShareSecret = "ocm-share-secret"

func TestExchangeCacheTTL(t *testing.T) {
	if exchangeTTLSafetyMargin != 60*time.Second {
		t.Fatalf("safety margin = %s, want 1m", exchangeTTLSafetyMargin)
	}
	if exchangeFailureTTL != 5*time.Minute {
		t.Fatalf("failure memory = %s, want 5m", exchangeFailureTTL)
	}
	if got := exchangeCacheTTL(3600); got != 3540*time.Second {
		t.Fatalf("exchangeCacheTTL(3600) = %s, want 59m", got)
	}
	if got := exchangeCacheTTL(120); got != 60*time.Second {
		t.Fatalf("exchangeCacheTTL(120) = %s, want 1m", got)
	}
	if got := exchangeCacheTTL(60); got != time.Second {
		t.Fatalf("exchangeCacheTTL(60) = %s, want 1s", got)
	}
	if got := exchangeCacheTTL(0); got != time.Second {
		t.Fatalf("exchangeCacheTTL(0) = %s, want 1s", got)
	}
	if got := exchangeCacheTTL(-5); got != time.Second {
		t.Fatalf("exchangeCacheTTL(-5) = %s, want 1s", got)
	}
}

type scriptedSender struct {
	mu sync.Mutex

	discoveryCalls int
	tokenCalls     int
	auths          []string
	methods        []string
	accessTokens   []string

	capabilities       []string
	tokenEndpoint      string
	omitTokenEndpoint  bool
	tokenStatus        int
	expiresIn          int64
	rejectBearerSecret bool
	rejectLegacy       bool // denies the Bearer shared secret and the Basic header
	rejectAccessToken  bool
}

func (s *scriptedSender) snapshot() (discovery, tokens int, auths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discoveryCalls, s.tokenCalls, append([]string{}, s.auths...)
}

func (s *scriptedSender) snapshotWire() (discovery, tokens int, auths, methods []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.discoveryCalls, s.tokenCalls, append([]string{}, s.auths...), append([]string{}, s.methods...)
}

func (s *scriptedSender) lastToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.accessTokens) == 0 {
		return ""
	}
	return s.accessTokens[len(s.accessTokens)-1]
}

func (s *scriptedSender) serve(t *testing.T, srv *httptest.Server, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	switch r.URL.Path {
	case "/.well-known/ocm":
		s.mu.Lock()
		s.discoveryCalls++
		caps := s.capabilities
		endpoint := s.tokenEndpoint
		omit := s.omitTokenEndpoint
		s.mu.Unlock()
		if caps == nil {
			caps = []string{"exchange-token"}
		}
		if endpoint == "" {
			endpoint = srv.URL + "/ocm/token"
		}
		payload := map[string]any{
			"enabled":       true,
			"apiVersion":    "1.2.0",
			"endPoint":      srv.URL + "/ocm",
			"provider":      "reva",
			"resourceTypes": []any{},
			"capabilities":  caps,
		}
		if !omit {
			payload["tokenEndPoint"] = endpoint
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode discovery: %v", err)
		}
	case "/ocm/token":
		s.mu.Lock()
		s.tokenCalls++
		call := s.tokenCalls
		status := s.tokenStatus
		expires := s.expiresIn
		s.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		if status != http.StatusOK {
			http.Error(w, "token exchange failed", status)
			return
		}
		if expires == 0 {
			expires = 3600
		}
		token := fmt.Sprintf("ocm-access-token-%d", call)
		s.mu.Lock()
		s.accessTokens = append(s.accessTokens, token)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   expires,
		}); err != nil {
			t.Errorf("encode token: %v", err)
		}
	default:
		auth := r.Header.Get("Authorization")
		s.mu.Lock()
		s.auths = append(s.auths, auth)
		s.methods = append(s.methods, r.Method)
		rejectToken := s.rejectAccessToken
		rejectSecret := s.rejectBearerSecret
		rejectLegacy := s.rejectLegacy
		s.mu.Unlock()
		if strings.HasPrefix(auth, "Bearer ocm-access-token-") {
			if rejectToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeScriptedDAV(w, r)
			return
		}
		if auth == "Bearer "+ocmShareSecret {
			if rejectSecret || rejectLegacy {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeScriptedDAV(w, r)
			return
		}
		if auth == legacyBasicHeader(ocmShareSecret) {
			if rejectLegacy {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeScriptedDAV(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func writeScriptedDAV(w http.ResponseWriter, r *http.Request) {
	if r.Method == "PROPFIND" {
		writeReceivedWebDAVPropfind(w, true)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func legacyBasicHeader(secret string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(secret+":"))
}

func startScriptedSender(t *testing.T, script *scriptedSender) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		script.serve(t, srv, w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func scriptedShare(srv *httptest.Server, strict bool) *ocmpb.ReceivedShare {
	var share *ocmpb.ReceivedShare
	if strict {
		share = testCodeFlowReceivedShare(srv.Listener.Addr().String(), srv.URL)
	} else {
		share = testReceivedShare(srv.Listener.Addr().String(), "share-abc", false)
	}
	share.Protocols[0].GetWebdavOptions().SharedSecret = ocmShareSecret
	return share
}

func loggedContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.TraceLevel)
	return appctx.WithLogger(context.Background(), &logger), &buf
}

func assertLogMode(t *testing.T, buf *bytes.Buffer, mode string) {
	t.Helper()
	text := buf.String()
	if !strings.Contains(text, `"mode":"`+mode+`"`) {
		t.Fatalf("logs missing mode %s: %s", mode, text)
	}
	if strings.Contains(text, ocmShareSecret) || strings.Contains(text, "ocm-access-token") {
		t.Fatalf("logs leaked credentials: %s", text)
	}
}

func assertNoLogMode(t *testing.T, buf *bytes.Buffer, mode string) {
	t.Helper()
	text := buf.String()
	if strings.Contains(text, `"mode":"`+mode+`"`) {
		t.Fatalf("logs contain mode %s: %s", mode, text)
	}
	if strings.Contains(text, ocmShareSecret) || strings.Contains(text, "ocm-access-token") {
		t.Fatalf("logs leaked credentials: %s", text)
	}
}

func TestOpportunisticTokenExchange(t *testing.T) {
	ref := &provider.Reference{Path: "/share-abc"}

	t.Run("exchange offered and working", func(t *testing.T) {
		script := &scriptedSender{}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		c1, _, _, err := d.webdavClient(ctx, ref)
		if err != nil {
			t.Fatalf("webdavClient: %v", err)
		}
		info, err := c1.Stat("")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("expected a collection")
		}
		token := script.lastToken()
		if token != "ocm-access-token-1" {
			t.Fatalf("access token = %q", token)
		}
		cc, ok := d.cachedShareClient(share.GetId().GetOpaqueId())
		if !ok || cc.mode != modeExchange {
			t.Fatalf("cached mode = %q, ok = %v", ccMode(cc), ok)
		}
		if cc.authHeader != "Bearer "+token {
			t.Fatalf("cached auth = %q", cc.authHeader)
		}
		if cc.client != c1 {
			t.Fatal("cached client is not the returned client")
		}
		gotTTL := time.Until(cc.expiresAt)
		wantTTL := exchangeCacheTTL(3600)
		if gotTTL < wantTTL-2*time.Second || gotTTL > wantTTL {
			t.Fatalf("cached for %s, want about %s", gotTTL, wantTTL)
		}
		_, tokenCalls, auths := script.snapshot()
		if tokenCalls != 1 {
			t.Fatalf("token calls = %d, want 1", tokenCalls)
		}
		if !containsAuth(auths, "Bearer "+token) {
			t.Fatalf("Stat did not send the access token: %v", auths)
		}
		if containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatal("Stat sent the shared secret")
		}

		c2, _, _, err := d.webdavClient(ctx, ref)
		if err != nil {
			t.Fatalf("cached webdavClient: %v", err)
		}
		if c2 != c1 {
			t.Fatal("expected the exchanged client to be reused")
		}
		if _, tokenCalls, _ = script.snapshot(); tokenCalls != 1 {
			t.Fatalf("token calls after cache hit = %d, want 1", tokenCalls)
		}
		assertLogMode(t, logs, modeExchange)

		cc.expiresAt = time.Now().Add(-time.Millisecond)
		c3, _, _, err := d.webdavClient(ctx, ref)
		if err != nil {
			t.Fatalf("webdavClient after expiry: %v", err)
		}
		if c3 == c1 {
			t.Fatal("expected a new client after token expiry")
		}
		if _, tokenCalls, _ = script.snapshot(); tokenCalls != 2 {
			t.Fatalf("token calls after expiry = %d, want 2", tokenCalls)
		}
		if c3Auth(t, d, share) != "Bearer "+script.lastToken() {
			t.Fatalf("auth after expiry = %q", c3Auth(t, d, share))
		}

		dUp := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		_, before, _ := script.snapshot()
		auth, err := dUp.uploadAuth(ctx, share, share.Protocols[0].GetWebdavOptions().Uri, ocmShareSecret, share.GetId())
		if err != nil {
			t.Fatalf("uploadAuth: %v", err)
		}
		if auth != "Bearer "+script.lastToken() {
			t.Fatalf("uploadAuth = %q", auth)
		}
		_, after, _ := script.snapshot()
		if after != before+1 {
			t.Fatalf("upload token calls = %d, want %d", after-before, 1)
		}
		auth2, err := dUp.uploadAuth(ctx, share, share.Protocols[0].GetWebdavOptions().Uri, ocmShareSecret, share.GetId())
		if err != nil {
			t.Fatalf("cached uploadAuth: %v", err)
		}
		if auth2 != auth {
			t.Fatalf("cached uploadAuth = %q, want %q", auth2, auth)
		}
		if _, after2, _ := script.snapshot(); after2 != after {
			t.Fatalf("cached upload exchanged again: calls %d then %d", after, after2)
		}
		assertLogMode(t, logs, modeExchange)
	})

	t.Run("exchange offered but failing", func(t *testing.T) {
		script := &scriptedSender{tokenStatus: http.StatusInternalServerError}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("webdavClient: %v", err)
		}
		discovery, tokens, auths := script.snapshot()
		if discovery != 1 || tokens != 1 {
			t.Fatalf("discovery = %d token = %d, want 1 and 1", discovery, tokens)
		}
		if !containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer was not used: %v", auths)
		}
		if !d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("exchange failure was not remembered")
		}
		_, ttl, err := d.exchangeMiss.GetWithTTL(share.GetId().GetOpaqueId())
		if err != nil {
			t.Fatalf("failure memory: %v", err)
		}
		if ttl < exchangeFailureTTL-2*time.Second || ttl > exchangeFailureTTL {
			t.Fatalf("failure memory ttl = %s, want about %s", ttl, exchangeFailureTTL)
		}
		assertLogMode(t, logs, modeBearer)

		_ = d.ccache.Remove(share.GetId().GetOpaqueId())
		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("second webdavClient: %v", err)
		}
		_, tokens, _ = script.snapshot()
		if tokens != 1 {
			t.Fatalf("token calls after remembered failure = %d, want 1", tokens)
		}
	})

	t.Run("failure memory expires", func(t *testing.T) {
		prev := exchangeFailureTTL
		exchangeFailureTTL = 2 * time.Second
		t.Cleanup(func() { exchangeFailureTTL = prev })

		script := &scriptedSender{tokenStatus: http.StatusInternalServerError}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, _ := loggedContext(t)
		shareID := share.GetId().GetOpaqueId()

		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("webdavClient: %v", err)
		}
		_ = d.ccache.Remove(shareID)
		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("second webdavClient: %v", err)
		}
		if _, tokens, _ := script.snapshot(); tokens != 1 {
			t.Fatalf("token calls while failure is remembered = %d, want 1", tokens)
		}

		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) && d.exchangeRememberedDown(shareID) {
			time.Sleep(50 * time.Millisecond)
		}
		if d.exchangeRememberedDown(shareID) {
			t.Fatal("failure memory did not expire")
		}
		_ = d.ccache.Remove(shareID)
		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("webdavClient after failure expiry: %v", err)
		}
		if _, tokens, _ := script.snapshot(); tokens != 2 {
			t.Fatalf("token calls after failure expiry = %d, want 2", tokens)
		}
	})

	t.Run("no exchange-token capability", func(t *testing.T) {
		script := &scriptedSender{capabilities: []string{"invites"}}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("webdavClient: %v", err)
		}
		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("second webdavClient: %v", err)
		}
		discovery, tokens, auths := script.snapshot()
		if tokens != 0 {
			t.Fatalf("token calls = %d, want 0", tokens)
		}
		if discovery != 1 {
			t.Fatalf("discovery calls = %d, want 1", discovery)
		}
		if !containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer was not used: %v", auths)
		}
		if d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("missing capability was remembered as a failure")
		}
		assertLogMode(t, logs, modeBearer)
		assertNoLogMode(t, logs, modeExchange)
	})

	t.Run("no capability uses basic legacy", func(t *testing.T) {
		script := &scriptedSender{
			capabilities:       []string{"invites"},
			rejectBearerSecret: true,
		}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
			t.Fatalf("webdavClient: %v", err)
		}
		_, tokens, auths := script.snapshot()
		if tokens != 0 {
			t.Fatalf("token calls = %d, want 0", tokens)
		}
		if !containsAuth(auths, legacyBasicHeader(ocmShareSecret)) {
			t.Fatalf("basic legacy was not used: %v", auths)
		}
		assertLogMode(t, logs, modeLegacy)
	})

	t.Run("no token endpoint is cached as not offered", func(t *testing.T) {
		script := &scriptedSender{
			omitTokenEndpoint: true,
			capabilities:      []string{"exchange-token"},
		}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)
		shareID := share.GetId().GetOpaqueId()
		endpoint := share.Protocols[0].GetWebdavOptions().Uri

		for range 2 {
			auth, err := d.uploadAuth(ctx, share, endpoint, ocmShareSecret, share.GetId())
			if err != nil {
				t.Fatalf("uploadAuth: %v", err)
			}
			if auth != "Bearer "+ocmShareSecret {
				t.Fatalf("uploadAuth = %q, want legacy bearer", auth)
			}
		}
		// Drop any legacy client so the next WebDAV open is cold and would
		// rediscover a sender that was not negatively cached.
		for range 2 {
			_ = d.ccache.Remove(shareID)
			if _, _, _, err := d.webdavClient(ctx, ref); err != nil {
				t.Fatalf("webdavClient: %v", err)
			}
		}

		discovery, tokens, auths := script.snapshot()
		if discovery != 1 {
			t.Fatalf("discovery calls = %d, want 1", discovery)
		}
		if tokens != 0 {
			t.Fatalf("token calls = %d, want 0", tokens)
		}
		if !containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer was not used: %v", auths)
		}
		if d.exchangeRememberedDown(shareID) {
			t.Fatal("missing token endpoint was remembered as a failure")
		}
		assertLogMode(t, logs, modeBearer)
		assertNoLogMode(t, logs, modeExchange)

		strictShare := scriptedShare(srv, true)
		got, err := d.getTokenEndpoint(ctx, strictShare)
		if got != "" {
			t.Fatalf("getTokenEndpoint = %q, want no endpoint", got)
		}
		if _, ok := err.(errtypes.IsNotFound); !ok {
			t.Fatalf("getTokenEndpoint error = %T %v, want NotFound", err, err)
		}
		wantErr := "error: not found: sender discovery at " +
			strings.TrimSpace(strictShare.GetCreator().GetIdp()) +
			" has no tokenEndPoint"
		if err.Error() != wantErr {
			t.Fatalf("getTokenEndpoint error = %q, want %q", err.Error(), wantErr)
		}
		discovery, tokens, _ = script.snapshot()
		if discovery != 1 || tokens != 0 {
			t.Fatalf("after strict lookup discovery = %d token = %d, want 1 and 0", discovery, tokens)
		}
	})

	t.Run("policy violation", func(t *testing.T) {
		script := &scriptedSender{tokenEndpoint: "https://10.9.8.7/ocm/token"}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		for range 2 {
			_, _, _, err := d.webdavClient(ctx, ref)
			if !errors.Is(err, client.ErrPolicyViolation) {
				t.Fatalf("webdavClient error = %v, want ErrPolicyViolation", err)
			}
		}
		_, tokens, auths := script.snapshot()
		if tokens != 0 {
			t.Fatalf("token calls = %d, want 0", tokens)
		}
		if len(auths) != 0 {
			t.Fatalf("legacy WebDAV was contacted: %v", auths)
		}
		if d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("policy violation was remembered as an exchange failure")
		}
		assertNoLogMode(t, logs, modeBearer)
		assertNoLogMode(t, logs, modeLegacy)
		assertNoLogMode(t, logs, modeExchange)
	})

	t.Run("rejected access token falls back", func(t *testing.T) {
		script := &scriptedSender{rejectAccessToken: true}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		err := d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
			_, statErr := c.Stat(rel)
			return statErr
		})
		if err != nil {
			t.Fatalf("withExchangeRetry: %v", err)
		}
		_, tokens, auths := script.snapshot()
		if tokens != 2 {
			t.Fatalf("token calls = %d, want 2", tokens)
		}
		if !containsAuth(auths, "Bearer ocm-access-token-1") || !containsAuth(auths, "Bearer ocm-access-token-2") {
			t.Fatalf("access tokens were not both challenged: %v", auths)
		}
		if !containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer was not used after rejection: %v", auths)
		}
		if !d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("rejected token was not remembered")
		}
		assertLogMode(t, logs, modeExchange)
		assertLogMode(t, logs, modeBearer)

		before := tokens
		err = d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
			_, statErr := c.Stat(rel)
			return statErr
		})
		if err != nil {
			t.Fatalf("cached legacy retry: %v", err)
		}
		if _, tokens, _ = script.snapshot(); tokens != before {
			t.Fatalf("token calls after fallback = %d, want %d", tokens, before)
		}

		dUp := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		_, before, _ = script.snapshot()
		uploadRef := &provider.Reference{Path: "/share-abc/file.txt"}
		body := io.NopCloser(bytes.NewReader([]byte("hi")))
		err = dUp.Upload(ctx, uploadRef, body, nil)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		_, after, _ := script.snapshot()
		if after != before+2 {
			t.Fatalf("upload token calls = %d, want 2", after-before)
		}
		if !dUp.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("upload did not remember the rejected token")
		}
		err = dUp.Upload(
			ctx,
			uploadRef,
			io.NopCloser(bytes.NewReader([]byte("again"))),
			nil,
		)
		if err != nil {
			t.Fatalf("second Upload: %v", err)
		}
		if _, after2, _ := script.snapshot(); after2 != after {
			t.Fatalf("second upload token calls = %d, want %d", after2, after)
		}
		assertLogMode(t, logs, modeBearer)
	})

	t.Run("basic-only upload after exchange failure", func(t *testing.T) {
		script := &scriptedSender{
			tokenStatus:        http.StatusInternalServerError,
			rejectBearerSecret: true,
		}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		err := d.Upload(
			ctx,
			&provider.Reference{Path: "/share-abc/file.txt"},
			io.NopCloser(bytes.NewReader([]byte("hi"))),
			nil,
		)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		discovery, tokens, auths, methods := script.snapshotWire()
		if discovery != 1 || tokens != 1 {
			t.Fatalf("discovery = %d token = %d, want 1 and 1", discovery, tokens)
		}
		if containsAuth(auths, "Bearer ocm-access-token-1") {
			t.Fatalf("failed exchange sent an access token: %v", auths)
		}
		assertBearerThenBasicUpload(t, methods, auths)
		if !d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("exchange failure was not remembered")
		}
		assertLogMode(t, logs, modeLegacy)
		assertNoLogMode(t, logs, modeBearer)
		assertNoLogMode(t, logs, modeExchange)
	})

	t.Run("basic-only upload after token rejection", func(t *testing.T) {
		script := &scriptedSender{
			rejectAccessToken:  true,
			rejectBearerSecret: true,
		}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		err := d.Upload(
			ctx,
			&provider.Reference{Path: "/share-abc/file.txt"},
			io.NopCloser(bytes.NewReader([]byte("hi"))),
			nil,
		)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		// The first 401 re-exchanges once. The final attempt remembers that
		// rejection and selects Basic through the legacy probe.
		_, tokens, auths, methods := script.snapshotWire()
		if tokens != 2 {
			t.Fatalf("token calls = %d, want 2", tokens)
		}
		token1 := firstAuthIndex(methods, auths, http.MethodPut, "Bearer ocm-access-token-1")
		token2 := firstAuthIndex(methods, auths, http.MethodPut, "Bearer ocm-access-token-2")
		bearer := firstAuthIndex(methods, auths, "PROPFIND", "Bearer "+ocmShareSecret)
		if token1 < 0 || token2 < token1 || bearer < token2 {
			t.Fatalf(
				"post-401 order token1=%d token2=%d bearer=%d methods=%v auths=%v",
				token1,
				token2,
				bearer,
				methods,
				auths,
			)
		}
		assertBearerThenBasicUpload(t, methods, auths)
		if !d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
			t.Fatal("rejected token was not remembered")
		}
		assertLogMode(t, logs, modeExchange)
		assertLogMode(t, logs, modeLegacy)
		assertNoLogMode(t, logs, modeBearer)
	})

	t.Run("legacy rejection after exchange is invalid credentials", func(t *testing.T) {
		script := &scriptedSender{
			rejectAccessToken: true,
			rejectLegacy:      true,
		}
		srv := startScriptedSender(t, script)
		share := scriptedShare(srv, false)
		d := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		ctx, logs := loggedContext(t)

		assertDenied := func(t *testing.T, err error) {
			t.Helper()
			if err == nil {
				t.Fatal("expected InvalidCredentials, got success")
			}
			if _, ok := err.(errtypes.IsInvalidCredentials); !ok {
				t.Fatalf("error = %T %v, want InvalidCredentials", err, err)
			}
			if gowebdav.IsErrCode(err, http.StatusUnauthorized) {
				t.Fatalf("raw WebDAV 401: %v", err)
			}
		}

		err := d.withExchangeRetry(ctx, ref, func(c *gowebdav.Client, rel string) error {
			_, statErr := c.Stat(rel)
			return statErr
		})
		assertDenied(t, err)
		_, tokens, auths := script.snapshot()
		if tokens != 2 {
			t.Fatalf("token calls = %d, want 2", tokens)
		}
		if !containsAuth(auths, "Bearer ocm-access-token-1") || !containsAuth(auths, "Bearer ocm-access-token-2") {
			t.Fatalf("exchanged tokens were not both challenged: %v", auths)
		}
		if !containsAuth(auths, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer was not challenged: %v", auths)
		}
		if !containsAuth(auths, legacyBasicHeader(ocmShareSecret)) {
			t.Fatalf("legacy basic was not challenged: %v", auths)
		}

		dUp := newTestReceivedDriver(t)
		stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share}})
		_, before, beforeAuths, beforeMethods := script.snapshotWire()
		uploadRef := &provider.Reference{Path: "/share-abc/file.txt"}
		err = dUp.Upload(ctx, uploadRef, io.NopCloser(bytes.NewReader([]byte("hi"))), nil)
		assertDenied(t, err)
		if !strings.Contains(err.Error(), "error accessing OCM share") {
			t.Fatalf("upload error = %q, want legacy probe denial", err)
		}
		_, after, afterAuths, afterMethods := script.snapshotWire()
		if after != before+2 {
			t.Fatalf("upload token calls = %d, want 2", after-before)
		}
		if countAuth(afterAuths, "Bearer "+ocmShareSecret) <= countAuth(beforeAuths, "Bearer "+ocmShareSecret) {
			t.Fatalf("upload did not challenge the legacy bearer: %v", afterAuths)
		}
		basic := legacyBasicHeader(ocmShareSecret)
		if countAuth(afterAuths, basic) <= countAuth(beforeAuths, basic) {
			t.Fatalf("upload did not challenge the legacy basic: %v", afterAuths)
		}
		if countMethodAuth(afterMethods, afterAuths, http.MethodPut, "Bearer "+ocmShareSecret) !=
			countMethodAuth(beforeMethods, beforeAuths, http.MethodPut, "Bearer "+ocmShareSecret) {
			t.Fatalf("legacy bearer write ran after probe denial: methods=%v auths=%v", afterMethods, afterAuths)
		}
		if countMethodAuth(afterMethods, afterAuths, http.MethodPut, basic) !=
			countMethodAuth(beforeMethods, beforeAuths, http.MethodPut, basic) {
			t.Fatalf("legacy basic write ran after probe denial: methods=%v auths=%v", afterMethods, afterAuths)
		}
		assertLogMode(t, logs, modeExchange)
	})

	t.Run("strict share unchanged", func(t *testing.T) {
		t.Run("failed exchange does not fall back", func(t *testing.T) {
			script := &scriptedSender{tokenStatus: http.StatusInternalServerError}
			srv := startScriptedSender(t, script)
			share := scriptedShare(srv, true)
			d := newTestReceivedDriver(t)
			stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share, share}})
			ctx, logs := loggedContext(t)

			for range 2 {
				_, _, _, err := d.webdavClient(ctx, ref)
				if err == nil {
					t.Fatal("expected token exchange to fail")
				}
			}
			_, tokens, auths := script.snapshot()
			if tokens != 2 {
				t.Fatalf("token calls = %d, want 2", tokens)
			}
			if len(auths) != 0 {
				t.Fatalf("strict failure contacted WebDAV: %v", auths)
			}
			if d.exchangeRememberedDown(share.GetId().GetOpaqueId()) {
				t.Fatal("strict share remembered an exchange failure")
			}
			assertNoLogMode(t, logs, modeBearer)
			assertNoLogMode(t, logs, modeLegacy)
			assertNoLogMode(t, logs, modeExchange)
		})

		t.Run("success is not cached", func(t *testing.T) {
			script := &scriptedSender{}
			srv := startScriptedSender(t, script)
			share := scriptedShare(srv, true)
			d := newTestReceivedDriver(t)
			stampGateway(&mockReceivedGateway{shares: []*ocmpb.ReceivedShare{share, share}})
			ctx, _ := loggedContext(t)

			c1, _, _, err := d.webdavClient(ctx, ref)
			if err != nil {
				t.Fatalf("first webdavClient: %v", err)
			}
			c2, _, _, err := d.webdavClient(ctx, ref)
			if err != nil {
				t.Fatalf("second webdavClient: %v", err)
			}
			if c1 == c2 {
				t.Fatal("strict path reused a cached client")
			}
			if _, tokens, _ := script.snapshot(); tokens != 2 {
				t.Fatalf("token calls = %d, want 2", tokens)
			}
			if _, ok := d.cachedShareClient(share.GetId().GetOpaqueId()); ok {
				t.Fatal("strict path cached a client")
			}
		})
	})
}

func ccMode(cc *cachedClient) string {
	if cc == nil {
		return ""
	}
	return cc.mode
}

func c3Auth(t *testing.T, d *driver, share *ocmpb.ReceivedShare) string {
	t.Helper()
	cc, ok := d.cachedShareClient(share.GetId().GetOpaqueId())
	if !ok {
		t.Fatal("expected a cached client")
	}
	return cc.authHeader
}

func containsAuth(auths []string, want string) bool {
	return countAuth(auths, want) > 0
}

func countAuth(auths []string, want string) int {
	n := 0
	for _, auth := range auths {
		if auth == want {
			n++
		}
	}
	return n
}

func countMethodAuth(methods, auths []string, method, auth string) int {
	n := 0
	for i, got := range auths {
		if i < len(methods) && methods[i] == method && got == auth {
			n++
		}
	}
	return n
}

func firstAuthIndex(methods, auths []string, method, auth string) int {
	for i, got := range auths {
		if i < len(methods) && methods[i] == method && got == auth {
			return i
		}
	}
	return -1
}

func assertBearerThenBasicUpload(t *testing.T, methods, auths []string) {
	t.Helper()
	bearer := "Bearer " + ocmShareSecret
	basic := legacyBasicHeader(ocmShareSecret)
	bearerProbe := firstAuthIndex(methods, auths, "PROPFIND", bearer)
	basicProbe := firstAuthIndex(methods, auths, "PROPFIND", basic)
	basicPut := firstAuthIndex(methods, auths, http.MethodPut, basic)
	if bearerProbe < 0 || basicProbe < bearerProbe || basicPut < basicProbe {
		t.Fatalf(
			"legacy selection order bearer=%d basicProbe=%d basicPut=%d methods=%v auths=%v",
			bearerProbe,
			basicProbe,
			basicPut,
			methods,
			auths,
		)
	}
	if countMethodAuth(methods, auths, http.MethodPut, bearer) != 0 {
		t.Fatalf("upload write used the bearer secret: methods=%v auths=%v", methods, auths)
	}
}
