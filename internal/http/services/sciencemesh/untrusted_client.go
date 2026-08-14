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

package sciencemesh

import (
	"context"
	"crypto/tls"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/wellknown"
	"github.com/cs3org/reva/v3/pkg/errtypes"
	"github.com/pkg/errors"
)

// ocmd.NewPublicOnlyClient is the shared constructor, but OCMClient.client is
// unexported and Discover hard-codes a 1 MiB cap. /discover rebuilds the same
// public-only transport here so OCMClientResponseLimit can bound the body.

const maxUntrustedRedirects = 3

const defaultDiscoverResponseLimit = 1 << 20

var errUntrustedNonHTTPS = stderrors.New("untrusted discover client requires https")

var errUntrustedTooManyRedirects = fmt.Errorf(
	"untrusted discover client stopped after %d redirects",
	maxUntrustedRedirects,
)

var errDiscoverResponseTooLarge = stderrors.New("discovery response body exceeds size limit")

func newUntrustedDiscoverClient(timeout time.Duration, insecure bool) *http.Client {
	var tr *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		tr = dt.Clone()
	} else {
		tr = &http.Transport{}
	}
	tr.Proxy = nil
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure,
		MinVersion:         tls.VersionTLS12,
	}
	tr.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   refuseNonPublicAddr,
	}).DialContext
	return &http.Client{
		Transport:     &publicOnlyTransport{Transport: tr},
		Timeout:       timeout,
		CheckRedirect: untrustedCheckRedirect,
	}
}

type publicOnlyTransport struct {
	*http.Transport
}

func (t *publicOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := requireUntrustedHTTPS(req.URL); err != nil {
		return nil, err
	}
	return t.Transport.RoundTrip(req)
}

func requireUntrustedHTTPS(u *url.URL) error {
	if u != nil && u.Scheme == "https" {
		return nil
	}
	scheme := ""
	if u != nil {
		scheme = u.Scheme
	}
	return fmt.Errorf("untrusted discover client refuses non-https scheme %q: %w", scheme, errUntrustedNonHTTPS)
}

func untrustedCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxUntrustedRedirects {
		return errUntrustedTooManyRedirects
	}
	return requireUntrustedHTTPS(req.URL)
}

func refuseNonPublicAddr(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !isPublicIP(ip) {
		return errtypes.BadRequest("refusing to connect to non-public address " + address)
	}
	return nil
}

// 64:ff9b::/96 embeds an IPv4 address in its low 32 bits (RFC 6052), so on a
// NAT64 network it can still reach internal hosts.
var nat64WellKnownPrefix = net.IPNet{IP: net.ParseIP("64:ff9b::"), Mask: net.CIDRMask(96, 128)}

func isPublicIP(ip net.IP) bool {
	if nat64WellKnownPrefix.Contains(ip) {
		v6 := ip.To16()
		return isPublicIP(net.IPv4(v6[12], v6[13], v6[14], v6[15]))
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	// 100.64.0.0/10 is carrier-grade NAT, which net.IP.IsPrivate does not cover
	if ip4 := ip.To4(); ip4 != nil && ip4[0] == 100 && ip4[1]&0xc0 == 64 {
		return false
	}
	return true
}

func discoverUntrusted(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	limit int64,
) (*wellknown.OcmDiscoveryData, error) {
	if limit <= 0 {
		limit = defaultDiscoverResponseLimit
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	remoteURL, err := url.JoinPath(endpoint, "/.well-known/ocm")
	if err != nil {
		return nil, err
	}
	body, err := discoverGET(
		ctx,
		client,
		remoteURL,
		limit,
	)
	if stderrors.Is(err, errDiscoverResponseTooLarge) {
		return nil, err
	}
	if err != nil || len(body) == 0 {
		remoteURL, err = url.JoinPath(endpoint, "/ocm-provider")
		if err != nil {
			return nil, err
		}
		body, err = discoverGET(
			ctx,
			client,
			remoteURL,
			limit,
		)
		if stderrors.Is(err, errDiscoverResponseTooLarge) {
			return nil, err
		}
		if err != nil {
			return nil, fmt.Errorf("Invalid response on OCM discovery: %w", err)
		}
		if len(body) == 0 {
			return nil, errtypes.InternalError("Invalid response on OCM discovery")
		}
	}

	var disco wellknown.OcmDiscoveryData
	if err := json.Unmarshal(body, &disco); err != nil {
		return nil, errtypes.InternalError("Invalid payload on OCM discovery")
	}
	return &disco, nil
}

func discoverGET(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error creating OCM discovery request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "error doing OCM discovery request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errtypes.InternalError("Remote does not offer a valid OCM discovery endpoint")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.Wrap(err, "malformed remote OCM discovery")
	}
	if int64(len(body)) > limit {
		return nil, errDiscoverResponseTooLarge
	}
	return body, nil
}
