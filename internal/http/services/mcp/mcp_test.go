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

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	authpb "github.com/cs3org/go-cs3apis/cs3/auth/provider/v1beta1"
	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	authmw "github.com/cs3org/reva/v3/internal/http/interceptors/auth"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/credential/loader"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/token/loader"
	_ "github.com/cs3org/reva/v3/internal/http/interceptors/auth/tokenwriter/loader"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/auth/scope"
	jwt "github.com/cs3org/reva/v3/pkg/token/manager/jwt"
	_ "github.com/cs3org/reva/v3/pkg/token/manager/loader"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The app passwords the fake auth middleware knows, by the bearer a test
// client sends.
var testPasswords = map[string]scope.AppAuthScope{
	"ro":     {PasswordID: "p-ro", Client: scope.ClientMCP, Permissions: scope.ReadOnlyPermissions()},
	"rw":     {PasswordID: "p-rw", Client: scope.ClientMCP, Permissions: scope.ReadWritePermissions()},
	"all":    {PasswordID: "p-all", Client: scope.ClientMCP, Permissions: scope.AppAuthPermissions()},
	"roots":  {PasswordID: "p-roots", Client: scope.ClientMCP, Permissions: scope.AppAuthPermissions(), Roots: []string{home + "/thesis"}},
	"nolist": {PasswordID: "p-nolist", Client: scope.ClientMCP, Permissions: []string{scope.PermFilesRead}},
	"webdav": {PasswordID: "p-webdav", Client: scope.ClientWebDAV, Permissions: scope.ReadOnlyPermissions()},
}

// syncBuffer collects the log lines of concurrent requests.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) lines() []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for l := range strings.Lines(b.b.String()) {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}

// fakeAuth puts in the request context what the auth middleware puts there,
// and writes the token to the response as its token writer does.
func fakeAuth(next http.Handler, logs io.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		var scopes map[string]*authpb.Scope
		if ap, ok := testPasswords[bearer]; ok {
			scopes, _ = scope.AddAppAuthScope(&ap, nil)
		} else if bearer == "web" {
			scopes, _ = scope.AddOwnerScope(nil)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		tkn := "jwt-" + bearer
		ctx := appctx.ContextSetUser(r.Context(), einstein)
		ctx = appctx.ContextSetToken(ctx, tkn)
		ctx = appctx.ContextSetScopes(ctx, scopes)
		ctx = metadata.AppendToOutgoingContext(ctx, appctx.TokenHeader, tkn)
		l := zerolog.New(logs)
		ctx = appctx.WithLogger(ctx, &l)
		w.Header().Set(appctx.TokenHeader, tkn)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type harness struct {
	t    *testing.T
	gw   *fakeGateway
	srv  *httptest.Server
	logs *syncBuffer
}

func newHarness(t *testing.T, conf map[string]any) *harness {
	t.Helper()
	gw := newFakeGateway(t)
	useGateway(gw)
	m := map[string]any{"web_base_url": "https://cernbox.example.org/"}
	maps.Copy(m, conf)
	s, err := New(context.Background(), m)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	h := &harness{t: t, gw: gw, logs: &syncBuffer{}}
	h.srv = httptest.NewServer(fakeAuth(s.Handler(), h.logs))
	t.Cleanup(h.srv.Close)
	return h
}

// headerTransport adds headers to every request of an MCP client.
type headerTransport struct {
	header http.Header
}

func (t headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range t.header {
		r.Header[k] = v
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connect(t *testing.T, url string, header http.Header, protocolVersion string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           &http.Client{Transport: headerTransport{header}},
		DisableStandaloneSSE: true,
	}, &sdk.ClientSessionOptions{ProtocolVersion: protocolVersion})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func (h *harness) connect(bearer string) *sdk.ClientSession {
	return connect(h.t, h.srv.URL, http.Header{"Authorization": {"Bearer " + bearer}}, "")
}

func callTool(t *testing.T, cs *sdk.ClientSession, name string, args any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// decode returns the structured result of a successful call.
func decode[T any](t *testing.T, res *sdk.CallToolResult) T {
	t.Helper()
	var out T
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content[0].(*sdk.TextContent).Text)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err == nil {
		err = json.Unmarshal(b, &out)
	}
	if err != nil {
		t.Fatalf("decoding %v: %v", res.StructuredContent, err)
	}
	return out
}

// failure returns the error of a failed call.
func failure(t *testing.T, res *sdk.CallToolResult) toolError {
	t.Helper()
	var e toolError
	if !res.IsError {
		t.Fatalf("expected an error, got %v", res.StructuredContent)
	}
	if err := json.Unmarshal([]byte(res.Content[0].(*sdk.TextContent).Text), &e); err != nil {
		t.Fatalf("error text %q: %v", res.Content[0].(*sdk.TextContent).Text, err)
	}
	return e
}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

// authGateway also authenticates one mcp app password, minting a real reva
// token for it.
type authGateway struct {
	*fakeGateway
	password string
	minted   string
}

const jwtSecret = "mcp-test-secret"

func (g *authGateway) Authenticate(ctx context.Context, req *gateway.AuthenticateRequest, _ ...grpc.CallOption) (*gateway.AuthenticateResponse, error) {
	if req.Type != "appauth" || req.ClientSecret != g.password {
		return &gateway.AuthenticateResponse{Status: &rpc.Status{Code: rpc.Code_CODE_UNAUTHENTICATED}}, nil
	}
	scopes, err := scope.AddAppAuthScope(&scope.AppAuthScope{PasswordID: "p-real", Client: scope.ClientMCP, Permissions: scope.ReadOnlyPermissions()}, nil)
	if err != nil {
		return nil, err
	}
	mgr, err := jwt.New(map[string]any{"secret": jwtSecret})
	if err != nil {
		return nil, err
	}
	if g.minted, err = mgr.MintToken(ctx, einstein, scopes); err != nil {
		return nil, err
	}
	return &gateway.AuthenticateResponse{Status: &rpc.Status{Code: rpc.Code_CODE_OK}, User: einstein, Token: g.minted}, nil
}

// The whole design rests on this: the context a tool handler receives carries
// the user, the scopes and the token the auth middleware put in the request
// context, so that service.Gateway(ctx) and the data transfers act as the
// user, with the app password's token.
func TestContextPropagation(t *testing.T) {
	gw := &authGateway{fakeGateway: newFakeGateway(t), password: strings.Repeat("Q", 43)}
	gw.Put(home+"/notes.txt", "hello")
	useGateway(gw)

	mw, err := authmw.New(map[string]any{
		"token_managers":   map[string]any{"jwt": map[string]any{"secret": jwtSecret}},
		"credential_chain": []string{"appauth"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.Handler())
	srv := httptest.NewServer(mw(mux))
	defer srv.Close()

	header := http.Header{"Authorization": {"Bearer " + gw.password}, "User-Agent": {"claude-code/2.0.1"}}
	for _, version := range []string{"2025-11-25", ""} {
		cs := connect(t, srv.URL+"/mcp", header, version)
		info := decode[entry](t, callTool(t, cs, toolStat, map[string]any{"path": home + "/notes.txt"}))
		if info.Path != home+"/notes.txt" || info.Size != 5 {
			t.Fatalf("protocol %q: stat %+v", version, info)
		}
		read := decode[readFileOut](t, callTool(t, cs, toolReadFile, map[string]any{"path": home + "/notes.txt"}))
		if read.Content != "hello" {
			t.Fatalf("protocol %q: read %+v", version, read)
		}
	}

	if gw.minted == "" {
		t.Fatal("the middleware never authenticated the app password")
	}
	tokens := gw.Tokens()
	if len(tokens) == 0 || slices.ContainsFunc(tokens, func(tkn string) bool { return tkn != gw.minted }) {
		t.Fatalf("every gateway call must carry the app password's reva token, got %q", tokens)
	}
	data := gw.DataTokens()
	if len(data) == 0 || slices.ContainsFunc(data, func(tkn string) bool { return tkn != gw.minted }) {
		t.Fatalf("every data transfer must carry the app password's reva token, got %q", data)
	}

	// The token writer sets the reva token on the response; the service takes
	// it off again.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req.Header = header.Clone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get(appctx.TokenHeader) != "" {
		t.Fatalf("tools/list: %d, %s: %q", resp.StatusCode, appctx.TokenHeader, resp.Header.Get(appctx.TokenHeader))
	}
}

// TestProtocol drives the handshake, the listing and a call through the SDK
// client, with the initialize handshake and with the stateless protocol.
func TestProtocol(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/a.txt", "alpha")
	for _, version := range []string{"2025-06-18", "2025-11-25", ""} {
		cs := connect(t, h.srv.URL, http.Header{"Authorization": {"Bearer ro"}}, version)
		if init := cs.InitializeResult(); init == nil || init.ServerInfo.Name != "cernbox" || !strings.Contains(init.Instructions, "list_spaces") {
			t.Fatalf("protocol %q: initialize result %+v", version, init)
		}
		if names := toolNames(t, cs); !slices.Contains(names, toolReadFile) {
			t.Fatalf("protocol %q: tools %v", version, names)
		}
		out := decode[readFileOut](t, callTool(t, cs, toolReadFile, map[string]any{"path": home + "/a.txt"}))
		if out.Content != "alpha" || out.ReturnedBytes != 5 || out.Truncated {
			t.Fatalf("protocol %q: read %+v", version, out)
		}
	}
}

func post(t *testing.T, url, bearer, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Protocol-Version", "2025-11-25")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

const listTools = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func TestIdentityGate(t *testing.T) {
	h := newHarness(t, nil)
	for _, bearer := range []string{"web", "webdav"} {
		resp := post(t, h.srv.URL, bearer, listTools)
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "an mcp app password is required") {
			t.Errorf("%s: got %d %q", bearer, resp.StatusCode, body)
		}
		if resp.Header.Get(appctx.TokenHeader) != "" {
			t.Errorf("%s: the reva token leaked in the response", bearer)
		}
	}
	if resp := post(t, h.srv.URL, "ro", listTools); resp.StatusCode != http.StatusOK || resp.Header.Get(appctx.TokenHeader) != "" {
		t.Errorf("mcp app password: got %d, token %q", resp.StatusCode, resp.Header.Get(appctx.TokenHeader))
	}
	if len(h.gw.Methods()) != 0 {
		t.Errorf("refused requests must not reach the gateway: %v", h.gw.Methods())
	}
}

func toolCall(name string, args string) string {
	return `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
}

func TestRateLimits(t *testing.T) {
	h := newHarness(t, map[string]any{"ratelimit": map[string]any{
		"calls_per_token_per_min":       3,
		"write_calls_per_token_per_min": 1,
	}})
	stat := toolCall(toolStat, `{"path":"`+home+`"}`)
	mkdir := func(n string) string { return toolCall(toolCreateFolder, `{"path":"`+home+`/`+n+`"}`) }

	if resp := post(t, h.srv.URL, "rw", mkdir("a")); resp.StatusCode != http.StatusOK {
		t.Fatalf("first write: %d", resp.StatusCode)
	}
	resp := post(t, h.srv.URL, "rw", mkdir("b"))
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("second write in a minute: %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	// A refused write is still a call: this is the third.
	if resp := post(t, h.srv.URL, "rw", stat); resp.StatusCode != http.StatusOK {
		t.Fatalf("a read after the write budget is spent: %d", resp.StatusCode)
	}
	if resp := post(t, h.srv.URL, "rw", stat); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("fourth call in a minute: %d", resp.StatusCode)
	}
	// The budget is the user's as well: another app password of the same user
	// gets nothing more.
	if resp := post(t, h.srv.URL, "ro", stat); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("another app password of the same user: %d", resp.StatusCode)
	}
	if resp := post(t, h.srv.URL, "rw", listTools); resp.StatusCode != http.StatusOK {
		t.Fatalf("listing tools is not a tool call: %d", resp.StatusCode)
	}
}

func TestReadBudget(t *testing.T) {
	h := newHarness(t, map[string]any{"ratelimit": map[string]any{"read_bytes_per_token_per_min": 8}})
	h.gw.Put(home+"/a.txt", "0123456789")
	cs := h.connect("ro")
	out := decode[readFileOut](t, callTool(t, cs, toolReadFile, map[string]any{"path": home + "/a.txt", "length": 6}))
	if out.Content != "012345" {
		t.Fatalf("first read: %+v", out)
	}
	if e := failure(t, callTool(t, cs, toolReadFile, map[string]any{"path": home + "/a.txt", "offset": 6, "length": 4})); e.Code != codeRateLimited {
		t.Fatalf("read past the budget: %+v", e)
	}
}

func TestAuditLog(t *testing.T) {
	h := newHarness(t, nil)
	h.gw.Put(home+"/secret.txt", "the content")
	cs := h.connect("ro")
	callTool(t, cs, toolReadFile, map[string]any{"path": home + "/secret.txt"})
	callTool(t, cs, toolWriteFile, map[string]any{"path": home + "/x.txt", "content": "nope"})

	var calls []map[string]any
	for _, l := range h.logs.lines() {
		if l["message"] == "mcp tool call" {
			calls = append(calls, l)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("one line per call, got %v", calls)
	}
	want := []map[string]any{
		{"password_id": "p-ro", "tool": toolReadFile, "path": home + "/secret.txt", "result": codeOK, "bytes": float64(11)},
		{"password_id": "p-ro", "tool": toolWriteFile, "result": codeNotPermittedByAP, "bytes": float64(0)},
	}
	for i, w := range want {
		for k, v := range w {
			if calls[i][k] != v {
				t.Errorf("call %d: %s = %v, want %v", i, k, calls[i][k], v)
			}
		}
		if _, ok := calls[i]["duration"]; !ok {
			t.Errorf("call %d: no duration", i)
		}
	}
	if strings.Contains(h.logs.b.String(), "the content") {
		t.Error("file content must never be logged")
	}
}
