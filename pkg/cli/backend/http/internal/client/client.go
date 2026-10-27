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

// Package client talks to CERNBox over HTTPS only: WebDAV and TUS (ocdav),
// Graph (ocgraph) and OCS. It never uses CS3 gRPC.
package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cs3org/reva/v3/internal/cli/cberr"
)

// Credential is a ready-to-send authentication header.
type Credential struct {
	Header string
	Value  string
}

// CredentialSource supplies a credential for each request. It must be safe for
// concurrent use, because the transfer engine sends requests in parallel.
type CredentialSource interface {
	Credential(ctx context.Context) (Credential, error)
}

// Renewer is a CredentialSource that can replace a credential the server
// refused. The client calls it once on a 401.
type Renewer interface {
	Renew(ctx context.Context, refused Credential) error
}

// CredentialFunc adapts a function to CredentialSource.
type CredentialFunc func(ctx context.Context) (Credential, error)

// Credential implements CredentialSource.
func (f CredentialFunc) Credential(ctx context.Context) (Credential, error) { return f(ctx) }

const (
	davFilesPrefix  = "/remote.php/dav/files"
	davSpacesPrefix = "/remote.php/dav/spaces"
	davTrashPrefix  = "/remote.php/dav/trash-bin"
	davMetaPrefix   = "/remote.php/dav/meta"
	graphBeta       = "/graph/v1beta1"
	graphV1         = "/graph/v1.0"
	ocsCapabilities = "/ocs/v1.php/cloud/capabilities"
)

// DefaultUserAgent identifies the CLI to the server. The build stamps the
// version in.
var DefaultUserAgent = "reva-cli/dev"

// Client is a CERNBox HTTP client. It is safe for concurrent use.
type Client struct {
	base      *url.URL
	hc        *http.Client
	creds     CredentialSource
	userAgent string
	maxRetry  int

	capsOnce sync.Once
	caps     *Capabilities
	capsErr  error

	meOnce sync.Once
	me     *User
	meErr  error

	spacesMu   sync.Mutex
	spaces     []Space
	spacesAt   time.Time
	spacesTTL  time.Duration
	spacesErr  error
	spacesOnce bool
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying HTTP client.
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// WithCredentials sets the credential source.
func WithCredentials(cs CredentialSource) Option { return func(c *Client) { c.creds = cs } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithMaxRetries sets how often a request is retried after a transient failure.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetry = n } }

// WithSpacesTTL sets how long the spaces listing is cached.
func WithSpacesTTL(d time.Duration) Option { return func(c *Client) { c.spacesTTL = d } }

// New returns a client for the CERNBox instance at endpoint.
func New(endpoint string, opts ...Option) (*Client, error) {
	if endpoint == "" {
		return nil, cberr.Usagef("no HTTPS endpoint configured: pass --endpoint")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, cberr.Usagef("invalid endpoint %q: %v", endpoint, err)
	}
	if u.Host == "" {
		return nil, cberr.Usagef("invalid endpoint %q: no host", endpoint)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")

	c := &Client{
		base:      u,
		hc:        &http.Client{Timeout: 0},
		userAgent: DefaultUserAgent,
		maxRetry:  3,
		spacesTTL: 5 * time.Minute,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// Endpoint returns the base URL the client was built with.
func (c *Client) Endpoint() string { return c.base.String() }

// URL builds an absolute URL from a server-absolute path that is already
// percent-encoded.
//
// Both Path and RawPath are set on purpose. With Path alone, url.URL.String
// escapes it again, and a space goes out as "%2520".
func (c *Client) URL(p string) string {
	u := *c.base
	raw := c.base.EscapedPath() + "/" + strings.TrimPrefix(p, "/")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		// Invalid escaping: treat the input as a literal path.
		u.Path = raw
		u.RawPath = ""
		return u.String()
	}
	u.Path = unescaped
	u.RawPath = raw
	return u.String()
}

// request is one HTTP call, described so that a retry can replay it.
type request struct {
	method  string
	url     string
	header  http.Header
	body    *requestBody
	op      string // verb used in error messages
	path    string // resource used in error messages
	noAuth  bool
	expects []int // acceptable status codes; empty means any 2xx

	// anyStatus accepts any answer. It is for a request made for its headers,
	// where a 401 or a 404 carries the answer too.
	anyStatus bool

	// used receives the credential of the last attempt, so that a renewal
	// replaces only that one.
	used *Credential
}

// requestBody is a body with its length. The transport ignores a hand-set
// Content-Length header and sends a body of unknown size chunked. reva's PUT
// handler rejects that with a 400.
type requestBody struct {
	// open is called once per attempt, so that a request can be replayed.
	open func() (io.ReadCloser, error)
	// length is -1 when unknown.
	length int64
}

func stringBody(s string) *requestBody {
	return &requestBody{
		open:   func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(s)), nil },
		length: int64(len(s)),
	}
}

func readerBody(open func() (io.ReadCloser, error), length int64) *requestBody {
	return &requestBody{open: open, length: length}
}

// do sends a request with credentials and retries. The caller must close the
// response body.
func (c *Client) do(ctx context.Context, r request) (*http.Response, error) {
	rn, ok := c.creds.(Renewer)
	if !ok || r.noAuth {
		return c.send(ctx, r)
	}
	var used Credential
	r.used = &used
	resp, err := c.send(ctx, r)
	if e, ok := errors.AsType[*cberr.Error](err); !ok || e.Status != http.StatusUnauthorized {
		return resp, err
	}
	if err := rn.Renew(ctx, used); err != nil {
		return nil, err
	}
	return c.send(ctx, r)
}

func (c *Client) send(ctx context.Context, r request) (*http.Response, error) {
	var lastErr error
	attempts := c.maxRetry + 1
	for attempt := range attempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}

		resp, err := c.attempt(ctx, r)
		if err != nil {
			if !retryable(err) {
				return nil, err
			}
			lastErr = err
			continue
		}

		if c.acceptable(r, resp.StatusCode) {
			return resp, nil
		}

		// Retry 5xx and 429 only. Other statuses are final.
		if isTransientStatus(resp.StatusCode) && attempt < attempts-1 {
			lastErr = c.statusError(r, resp)
			resp.Body.Close()
			continue
		}
		defer resp.Body.Close()
		return nil, c.statusError(r, resp)
	}
	if lastErr == nil {
		lastErr = errors.New("request failed")
	}
	return nil, lastErr
}

func (c *Client) attempt(ctx context.Context, r request) (*http.Response, error) {
	var body io.ReadCloser
	if r.body != nil {
		b, err := r.body.open()
		if err != nil {
			return nil, cberr.Wrap(cberr.KindOther, r.op, r.path, err)
		}
		body = b
	}
	// Close the body ourselves until Do takes it. An open file cannot be
	// deleted on Windows.
	handedOver := false
	defer func() {
		if body != nil && !handedOver {
			body.Close()
		}
	}()

	req, err := http.NewRequestWithContext(ctx, r.method, r.url, body)
	if err != nil {
		return nil, cberr.Wrap(cberr.KindOther, r.op, r.path, err)
	}
	if r.body != nil && r.body.length >= 0 {
		req.ContentLength = r.body.length
		if r.body.length == 0 {
			// Zero length with a non-nil body means unknown, so the transport
			// would send it chunked.
			req.Body = http.NoBody
		}
	}
	for k, vs := range r.header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("User-Agent", c.userAgent)

	if !r.noAuth {
		if c.creds == nil {
			return nil, cberr.Authf("no credentials available: sign in first")
		}
		cred, err := c.creds.Credential(ctx)
		if err != nil {
			return nil, err
		}
		if cred.Header != "" {
			req.Header.Set(cred.Header, cred.Value)
		}
		if r.used != nil {
			*r.used = cred
		}
	}

	// Do closes the body, except the http.NoBody that we swapped in.
	handedOver = req.Body != http.NoBody
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, transportError(r, err)
	}
	return resp, nil
}

func (c *Client) acceptable(r request, status int) bool {
	if r.anyStatus {
		return true
	}
	if len(r.expects) == 0 {
		return status >= 200 && status < 300
	}
	return slices.Contains(r.expects, status)
}

func (c *Client) statusError(r request, resp *http.Response) error {
	return cberr.FromStatus(resp.StatusCode, r.op, r.path, serverMessage(resp))
}

// serverMessage reads the message of an OCS JSON or DAV XML error body. It
// reads only a bounded prefix.
func serverMessage(resp *http.Response) string {
	const maxBody = 8 << 10
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil || len(b) == 0 {
		return ""
	}
	s := string(b)

	if m := extractTag(s, "s:message"); m != "" {
		return m
	}
	if m := extractTag(s, "message"); m != "" {
		return m
	}
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		return extractJSONField(s, "message")
	}
	return ""
}

func extractTag(s, tag string) string {
	open, closing := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(s[i:], closing)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[i+len(open) : i+j])
}

func extractJSONField(s, field string) string {
	key := `"` + field + `":`
	_, after, ok := strings.Cut(s, key)
	if !ok {
		return ""
	}
	rest := strings.TrimSpace(after)
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func transportError(r request, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if dnsErr, ok := errors.AsType[*net.DNSError](err); ok {
		return cberr.New(cberr.KindOther, r.op, r.path,
			fmt.Sprintf("cannot resolve %s: check the endpoint and your network", dnsErr.Name))
	}
	return cberr.Wrap(cberr.KindOther, r.op, r.path, err)
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// A retry does not fix an auth failure.
	if cberr.KindOf(err) == cberr.KindAuth {
		return false
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func isTransientStatus(status int) bool {
	return status == http.StatusTooManyRequests ||
		status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(1<<uint(attempt-1))*250*time.Millisecond, 5*time.Second)
}

func drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
