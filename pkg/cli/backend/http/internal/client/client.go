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

// Package client is the HTTP layer of the CLI. It talks to the same public
// CERNBox surface the web UI uses: WebDAV and TUS through ocdav, spaces and
// shares through ocgraph, feature discovery through OCS: and never speaks CS3
// gRPC, so it works from anywhere the service is reachable over HTTPS.
package client

import (
	"bytes"
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

// Credential is a ready-to-send authentication header. The auth layer produces
// it; the client only copies it onto the request, so that nothing here needs to
// know whether the token came from Kerberos, OIDC, or an app password.
type Credential struct {
	// Header is the header name, usually "Authorization".
	Header string
	// Value is the complete header value, for example "Bearer eyJ...".
	Value string
}

// CredentialSource supplies a credential for each request. Implementations are
// expected to refresh transparently and to be safe for concurrent use, since
// the transfer engine issues requests in parallel.
type CredentialSource interface {
	Credential(ctx context.Context) (Credential, error)
}

// Renewer is a CredentialSource that can replace a credential that the
// server refused. The client asks for it once on a 401, because a reva token
// can expire during a long run, or the server can refuse a cached one.
type Renewer interface {
	Renew(ctx context.Context, refused Credential) error
}

// CredentialFunc adapts a function to CredentialSource.
type CredentialFunc func(ctx context.Context) (Credential, error)

// Credential implements CredentialSource.
func (f CredentialFunc) Credential(ctx context.Context) (Credential, error) { return f(ctx) }

// Endpoint path prefixes on a CERNBox server.
const (
	davFilesPrefix  = "/remote.php/dav/files"
	davSpacesPrefix = "/remote.php/dav/spaces"
	davTrashPrefix  = "/remote.php/dav/trash-bin"
	davMetaPrefix   = "/remote.php/dav/meta"
	graphBeta       = "/graph/v1beta1"
	graphV1         = "/graph/v1.0"
	ocsCapabilities = "/ocs/v1.php/cloud/capabilities"
)

// DefaultUserAgent identifies the CLI to the server. The version is stamped in
// by cmd/cernbox so server-side telemetry can see the client version
// distribution, which is what makes it possible to time a deprecation.
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

// WithHTTPClient replaces the underlying HTTP client. Tests use it to inject a
// recording transport; the CLI uses it to set timeouts and TLS options.
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// WithCredentials sets the credential source.
func WithCredentials(cs CredentialSource) Option { return func(c *Client) { c.creds = cs } }

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithMaxRetries sets how many times an idempotent request is retried on a
// transient failure. Zero disables retrying.
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

// URL builds an absolute URL from a server-absolute path whose segments are
// already percent-encoded.
//
// Both Path and RawPath are set, and that is not redundant: url.URL.String
// renders RawPath only when it is a valid encoding of Path, and otherwise
// re-escapes Path. Setting Path alone to already-escaped text would send
// "%2520" for a space, which the server then decodes to a literal "%20" in the
// file name.
func (c *Client) URL(p string) string {
	u := *c.base
	raw := c.base.EscapedPath() + "/" + strings.TrimPrefix(p, "/")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		// Not valid escaping, so treat the input as a literal path and let
		// String do the encoding.
		u.Path = raw
		u.RawPath = ""
		return u.String()
	}
	u.Path = unescaped
	u.RawPath = raw
	return u.String()
}

// request is one HTTP call, described declaratively so that retry can replay it.
type request struct {
	method  string
	url     string
	header  http.Header
	body    *requestBody // nil for bodyless requests
	op      string       // verb used in error messages
	path    string       // resource used in error messages
	noAuth  bool
	expects []int // acceptable status codes; empty means any 2xx

	// noRetry marks a request that must not be replayed. The body field is a
	// factory precisely so that most requests can be, but a caller streaming
	// from a pipe or a non-seekable source cannot produce the bytes twice and
	// sets this.
	noRetry bool

	// anyStatus accepts whatever the server answers. It is for a request made
	// for its headers rather than its result, where a 401 or a 404 carries what
	// the caller came for just as a 200 does.
	anyStatus bool

	// used receives the credential of the last attempt, so that a renewal
	// replaces only that one.
	used *Credential
}

// requestBody is a body together with its length.
//
// The two travel together because they must not be separated. The transport
// ignores a Content-Length header set by hand and uses http.Request.
// ContentLength, falling back to chunked encoding for any body it cannot size
// itself: which is every body read from a file. reva's PUT handler reads the
// Content-Length header and rejects a request that has none, so a body without
// a length turns every upload into a 400. Making the length part of the body
// means a caller cannot supply one without the other.
type requestBody struct {
	// open returns a fresh reader. It is called once per attempt, which is what
	// makes a request replayable.
	open func() (io.ReadCloser, error)
	// length is the number of bytes open will produce, or -1 when unknown.
	length int64
}

// stringBody returns a body holding a string.
func stringBody(s string) *requestBody {
	return &requestBody{
		open:   func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(s)), nil },
		length: int64(len(s)),
	}
}

// bytesBody returns a body holding a byte slice.
func bytesBody(b []byte) *requestBody {
	return &requestBody{
		open:   func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil },
		length: int64(len(b)),
	}
}

// readerBody returns a body of known length produced by open.
func readerBody(open func() (io.ReadCloser, error), length int64) *requestBody {
	return &requestBody{open: open, length: length}
}

// do issues a request, attaching credentials and retrying transient failures.
// The caller owns the response body.
func (c *Client) do(ctx context.Context, r request) (*http.Response, error) {
	rn, ok := c.creds.(Renewer)
	if !ok || r.noAuth || r.noRetry {
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
			if !retryable(err) || r.noRetry {
				return nil, err
			}
			lastErr = err
			continue
		}

		if c.acceptable(r, resp.StatusCode) {
			return resp, nil
		}

		// A 5xx or 429 is worth another attempt; anything else is the server's
		// final answer and retrying would only delay the error.
		if isTransientStatus(resp.StatusCode) && !r.noRetry && attempt < attempts-1 {
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
	// The body is ours to close until the transport takes it, which it does only
	// in Do. Left open, a file being uploaded cannot be deleted on Windows, so
	// an outbox that removes what it sent would fail on an empty file.
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
			// A zero ContentLength with a non-nil body means "unknown" to the
			// transport, which then sends it chunked. http.NoBody is how an
			// empty body is stated explicitly.
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

	// Do closes the request body whatever happens, but an empty one was swapped
	// for http.NoBody and never reaches it.
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

// statusError turns a rejected response into a classified error, preferring any
// message the server supplied over the generic one.
func (c *Client) statusError(r request, resp *http.Response) error {
	msg := serverMessage(resp)
	return cberr.FromStatus(resp.StatusCode, r.op, r.path, msg)
}

// serverMessage extracts a short human message from an error response body.
// reva returns either an OCS JSON envelope or a DAV XML error; both carry a
// message worth showing, and neither is large, so reading a bounded prefix is
// cheap.
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
		if m := extractJSONField(s, "message"); m != "" {
			return m
		}
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

// transportError distinguishes a failure to reach the server from a failure of
// the request itself, because the advice differs: one is a network or DNS
// problem, the other is something the user can fix.
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
	// An authentication failure will not fix itself by being retried; the
	// credential source is responsible for refreshing.
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
	d := min(time.Duration(1<<uint(attempt-1))*250*time.Millisecond, 5*time.Second)
	return d
}

// drain consumes and closes a response body so the connection can be reused.
func drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}
