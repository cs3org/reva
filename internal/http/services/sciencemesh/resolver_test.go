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

package sciencemesh

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	ocmpb "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type launchCtxKey struct{}

const (
	launchSecret        = "share-secret-value"
	launchToken         = "access-token-value"
	launchCtxVal        = "launch-ctx"
	launchAuthorization = "Bearer launch-auth-token"
)

type observeClient struct {
	discoverErr       error
	exchangeErr       error
	discoverBare      bool
	token             string
	tokenByCall       []string
	endpointByCall    []string
	discoveryEndpoint string
	discoverCalls     int
	exchangeCalls     int
	origins           []string
	tokenURLs         []string
	codes             []string
	clientIDs         []string
	deadlines         int
	missingCtxValue   bool
}

func (c *observeClient) note(ctx context.Context) {
	if _, ok := ctx.Deadline(); ok {
		c.deadlines++
	}
	if ctx.Value(launchCtxKey{}) != launchCtxVal {
		c.missingCtxValue = true
	}
}

func (c *observeClient) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	c.discoverCalls++
	c.origins = append(c.origins, endpoint)
	c.note(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.discoverErr != nil {
		return nil, c.discoverErr
	}
	tokenEndpoint := "https://token.example/ocm/token"
	if n := len(c.endpointByCall); n > 0 {
		idx := c.discoverCalls - 1
		if idx >= n {
			idx = n - 1
		}
		tokenEndpoint = c.endpointByCall[idx]
	}
	base := c.discoveryEndpoint
	if base == "" {
		base = "https://sender.example/ocm"
	}
	caps := []string{"exchange-token"}
	if c.discoverBare {
		caps = []string{}
	}
	return &wellknown.OcmDiscoveryData{
		Endpoint:      base,
		TokenEndPoint: tokenEndpoint,
		Capabilities:  caps,
	}, nil
}

func (c *observeClient) ExchangeToken(
	ctx context.Context,
	tokenEndpoint, code, clientID string,
) (string, int64, error) {
	c.exchangeCalls++
	c.tokenURLs = append(c.tokenURLs, tokenEndpoint)
	c.codes = append(c.codes, code)
	c.clientIDs = append(c.clientIDs, clientID)
	c.note(ctx)
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	token := c.token
	if n := len(c.tokenByCall); n > 0 {
		idx := c.exchangeCalls - 1
		if idx >= n {
			idx = n - 1
		}
		token = c.tokenByCall[idx]
	}
	if c.exchangeErr != nil {
		return token, 0, c.exchangeErr
	}
	return token, 30, nil
}

// publicOnlyScript simulates a successful public discovery reply and sends
// every other hop through the real public-only client.
type publicOnlyScript struct {
	real          *ocmd.OCMClient
	tokenEndpoint string
	discoverCalls int
	exchangeCalls int
}

func (p *publicOnlyScript) Discover(ctx context.Context, endpoint string) (*wellknown.OcmDiscoveryData, error) {
	p.discoverCalls++
	if endpointIsNonPublic(endpoint) {
		return p.real.Discover(ctx, endpoint)
	}
	return &wellknown.OcmDiscoveryData{
		Endpoint:      "https://sender.example/ocm",
		TokenEndPoint: p.tokenEndpoint,
		Capabilities:  []string{"exchange-token"},
	}, nil
}

func (p *publicOnlyScript) ExchangeToken(
	ctx context.Context,
	tokenEndpoint, code, clientID string,
) (string, int64, error) {
	p.exchangeCalls++
	return p.real.ExchangeToken(ctx, tokenEndpoint, code, clientID)
}

func endpointIsNonPublic(endpoint string) bool {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

type clientConfigSeen struct {
	calls    int
	timeout  time.Duration
	insecure bool
}

type fakeReceivedGateway struct {
	gateway.GatewayAPIClient
	mu       sync.Mutex
	resp     *ocmpb.GetReceivedOCMShareResponse
	resps    []*ocmpb.GetReceivedOCMShareResponse
	err      error
	opaqueID string
	calls    int
	contexts []context.Context
}

func (f *fakeReceivedGateway) setResp(resp *ocmpb.GetReceivedOCMShareResponse) {
	f.mu.Lock()
	f.resp = resp
	f.mu.Unlock()
}

func (f *fakeReceivedGateway) GetReceivedOCMShare(
	ctx context.Context,
	req *ocmpb.GetReceivedOCMShareRequest,
	_ ...grpc.CallOption,
) (*ocmpb.GetReceivedOCMShareResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.contexts = append(f.contexts, ctx)
	if req != nil && req.GetRef() != nil && req.GetRef().GetId() != nil {
		f.opaqueID = req.GetRef().GetId().GetOpaqueId()
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(f.resps) > 0 {
		next := f.resps[0]
		f.resps = f.resps[1:]
		return next, nil
	}
	return f.resp, nil
}

type failingResponseWriter struct {
	header http.Header
	status int
	writes int
	body   string
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *failingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	w.body += string(p)
	return 0, errors.New("write failed")
}

type launchGatewayResolver struct {
	service.Clients
	mu sync.Mutex
	gw gateway.GatewayAPIClient
}

func (r *launchGatewayResolver) Gateway(context.Context) (gateway.GatewayAPIClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.gw == nil {
		return nil, errors.New("test gateway is not set")
	}
	return r.gw, nil
}

var (
	launchResolverOnce sync.Once
	launchResolver     = &launchGatewayResolver{}
)

func installLaunchGateway(t *testing.T, gw gateway.GatewayAPIClient) {
	t.Helper()
	launchResolverOnce.Do(func() {
		service.SetGlobal(launchResolver)
	})
	launchResolver.mu.Lock()
	launchResolver.gw = gw
	launchResolver.mu.Unlock()
}

func newTestHandler(t *testing.T, gw gateway.GatewayAPIClient, client launchClient) *appsHandler {
	t.Helper()
	installLaunchGateway(t, gw)
	h := &appsHandler{}
	if err := h.init(&config{
		OCMMountPoint:     "/ocm",
		ProviderDomain:    "receiver.example",
		OCMClientTimeout:  3,
		OCMClientInsecure: true,
	}); err != nil {
		t.Fatal(err)
	}
	if client != nil {
		h.newLaunchClient = func(time.Duration, bool) launchClient {
			return client
		}
	}
	targets := []string{"blank"}
	h.webappReceiveTargets = &targets
	return h
}

func newRecordingHandler(
	t *testing.T,
	gw gateway.GatewayAPIClient,
	client launchClient,
) (*appsHandler, *clientConfigSeen) {
	t.Helper()
	h := newTestHandler(t, gw, nil)
	seen := &clientConfigSeen{}
	h.newLaunchClient = func(timeout time.Duration, insecure bool) launchClient {
		seen.calls++
		seen.timeout = timeout
		seen.insecure = insecure
		_ = ocmd.NewPublicOnlyClient(timeout, insecure)
		return client
	}
	return h, seen
}

func newLaunchRequest(t *testing.T, filePath string) (*http.Request, *bytes.Buffer) {
	t.Helper()
	form := url.Values{}
	if filePath != "" {
		form.Set("file", filePath)
	}
	req := httptest.NewRequest(http.MethodPost, "/sciencemesh/open-in-app", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var logs bytes.Buffer
	logger := zerolog.New(&logs)
	ctx := appctx.WithLogger(req.Context(), &logger)
	ctx = context.WithValue(ctx, launchCtxKey{}, launchCtxVal)
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", launchAuthorization))
	return req.WithContext(ctx), &logs
}

func okShareResponse(share *ocmpb.ReceivedShare) *ocmpb.GetReceivedOCMShareResponse {
	return &ocmpb.GetReceivedOCMShareResponse{
		Status: &rpcv1beta1.Status{Code: rpcv1beta1.Code_CODE_OK},
		Share:  share,
	}
}

func receivedWebappShare(webdavURI, appURI, secret string, reqs []string) *ocmpb.ReceivedShare {
	protocols := []*ocmpb.Protocol{}
	if webdavURI != "" {
		protocols = append(protocols, webdavProtocol(webdavURI))
	}
	protocols = append(protocols, webappProtocol(appURI, secret, reqs))
	return &ocmpb.ReceivedShare{
		Id:        &ocmpb.ShareId{OpaqueId: "share-1"},
		Protocols: protocols,
	}
}

func webdavProtocol(uri string) *ocmpb.Protocol {
	return &ocmpb.Protocol{
		Term: &ocmpb.Protocol_WebdavOptions{
			WebdavOptions: &ocmpb.WebDAVProtocol{Uri: uri},
		},
	}
}

func webappProtocol(uri, secret string, reqs []string) *ocmpb.Protocol {
	return &ocmpb.Protocol{
		Term: &ocmpb.Protocol_WebappOptions{
			WebappOptions: &ocmpb.WebappProtocol{
				Uri:          uri,
				SharedSecret: secret,
				Requirements: reqs,
				Targets:      []string{"blank"},
			},
		},
	}
}

func assertLogsClean(t *testing.T, logs, secret, token string) {
	t.Helper()
	if secret != "" && strings.Contains(logs, secret) {
		t.Fatalf("log leaked shared secret: %s", logs)
	}
	if token != "" && strings.Contains(logs, token) {
		t.Fatalf("log leaked access token: %s", logs)
	}
}

func assertNotLeaked(t *testing.T, body, logs, secret, token string) {
	t.Helper()
	assertLogsClean(t, logs, secret, token)
	if secret != "" && strings.Contains(body, secret) {
		t.Fatalf("body leaked shared secret: %s", body)
	}
	if token != "" && strings.Contains(body, token) {
		t.Fatalf("body leaked access token: %s", body)
	}
}
