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

package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/cli/backend"
	grpcbackend "github.com/cs3org/reva/v3/pkg/cli/backend/grpc"
	httpbackend "github.com/cs3org/reva/v3/pkg/cli/backend/http"
	"github.com/cs3org/reva/v3/pkg/cli/data"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
)

// TransportAuto selects gRPC when a gateway is set, else HTTPS.
const TransportAuto = "auto"

// DefaultTransport is the built-in transport of the user plane, below the
// flag, the env, the config files and connection.transport of the brand.
var DefaultTransport = backend.GRPC

// gatewayConnectTimeout is short, so that in auto a gateway that cannot be
// reached gives Unavailable soon, and the command continues over HTTPS.
const gatewayConnectTimeout = 5 * time.Second

var _ backend.Backend = (*switcher)(nil)

// HTTPEndpoint returns the HTTPS endpoint of the deployment. Empty means
// none is known.
func (r *Runtime) HTTPEndpoint() string { return r.httpEndpoint }

// Transport returns the transport that the settings select: grpc, http or
// auto.
func (r *Runtime) Transport() string { return cmp.Or(r.transport, DefaultTransport) }

// UseBackend makes the user plane use b and d, for tests. A nil d is the
// data plane on the Locator of b.
func (r *Runtime) UseBackend(b backend.Backend, d backend.DataPlane) {
	r.backend, r.data = b, d
}

// Backend returns the metadata plane of the selected transport. It does not
// connect: the first call does. When an HTTPS endpoint is known, a call that
// the transport does not have goes over HTTPS, and so does the rest of the
// command when the gateway is unavailable in auto.
func (r *Runtime) Backend(ctx context.Context) (backend.Backend, error) {
	if r.backend != nil {
		return r.backend, nil
	}
	t := r.Transport()
	switch {
	case t == backend.GRPC, t == TransportAuto && r.gatewayAddr != "":
	case t == backend.HTTP, t == TransportAuto:
		b, err := r.httpBackend()
		if err != nil {
			return nil, err
		}
		r.backend = b
		return b, nil
	default:
		return nil, fmt.Errorf("unknown transport %q: use grpc, http or auto", t)
	}
	var b backend.Backend = grpcbackend.New(r.Gateway)
	if r.httpEndpoint != "" {
		b = &switcher{
			primary: b,
			auto:    t == TransportAuto,
			http:    r.httpBackend,
			note:    func(format string, args ...any) { r.Msg(os.Stderr, r.Name()+": "+format, args...) },
			warn:    r.Warnf,
		}
	}
	r.backend = b
	return b, nil
}

// httpBackend returns the backend of the HTTPS endpoint.
func (r *Runtime) httpBackend() (backend.Backend, error) {
	if r.httpB != nil {
		return r.httpB, nil
	}
	if r.httpEndpoint == "" {
		if r.Transport() == TransportAuto {
			return nil, fmt.Errorf("no gateway and no endpoint are set: pass --endpoint https://<host>%s, or --gateway%s", r.envHint("endpoint"), r.envHint("gateway"))
		}
		return nil, r.notSet("endpoint", "endpoint", "--endpoint", "endpoint https://<host>")
	}
	if strings.HasPrefix(r.httpEndpoint, "http://") && !r.insecure {
		return nil, fmt.Errorf("the endpoint %s has no TLS: pass --insecure to allow it (development only)", r.httpEndpoint)
	}
	b, err := httpbackend.New(r.httpEndpoint, httpbackend.Options{
		HTTPClient:  r.HTTPClient().GetNativeHTTP(),
		Credentials: revaCredentials{r},
		UserAgent:   r.Name() + "-cli/" + rversion.Version,
		MaxRetries:  -1,
	})
	if err != nil {
		return nil, err
	}
	r.httpB = b
	return b, nil
}

// revaCredentials sends the reva token as x-access-token, which every reva
// deployment takes. Authorization: Bearer works only where the bearer
// strategy is configured.
type revaCredentials struct{ r *Runtime }

func (c revaCredentials) Credential(ctx context.Context) (httpbackend.Credential, error) {
	t, err := c.r.Token(ctx)
	if err != nil {
		return httpbackend.Credential{}, err
	}
	return httpbackend.Credential{Header: appctx.TokenHeader, Value: t}, nil
}

func (c revaCredentials) Renew(ctx context.Context, refused httpbackend.Credential) error {
	if c.r.as != "" {
		return errors.New("the server refused the token of --as")
	}
	return c.r.renew(ctx, refused.Value)
}

// Data returns the data plane, on the Locator of the backend.
func (r *Runtime) Data(ctx context.Context) (backend.DataPlane, error) {
	if r.data != nil {
		return r.data, nil
	}
	b, err := r.Backend(ctx)
	if err != nil {
		return nil, err
	}
	p := data.New(r.HTTPClient().GetNativeHTTP(), b.Locator())
	p.ChunkSize = r.chunkSize()
	p.Renew = func(ctx context.Context) error {
		t, err := r.Token(ctx)
		if err != nil || r.as != "" {
			return errors.Join(err, errors.New("the server refused the token"))
		}
		return r.renew(ctx, t)
	}
	r.data = p
	return r.data, nil
}

// Ability returns the optional ability T of the backend, for example a
// backend.Searcher. When the backend does not have it and an HTTPS endpoint
// is known, it returns the one of the HTTPS backend, with a note. Else the
// error matches backend.ErrNotAvailable. what names the ability in the
// messages.
func Ability[T any](ctx context.Context, r *Runtime, what string) (T, error) {
	var zero T
	b, err := r.Backend(ctx)
	if err != nil {
		return zero, err
	}
	if s, ok := b.(*switcher); ok {
		b = s.current()
	}
	if a, ok := any(b).(T); ok {
		return a, nil
	}
	if r.httpEndpoint == "" || b.Transport() == backend.HTTP {
		return zero, backend.NotAvailable(b.Transport(), what)
	}
	h, err := r.httpBackend()
	if err != nil {
		return zero, fmt.Errorf("%w, and https cannot be used: %w", backend.NotAvailable(b.Transport(), what), err)
	}
	a, ok := any(h).(T)
	if !ok {
		return zero, backend.NotAvailable(h.Transport(), what)
	}
	r.Msg(os.Stderr, r.Name()+": %s is not available over %s: using https for it", what, b.Transport())
	return a, nil
}

// gatewayDialOption gives the connection to the gateway a short deadline.
func gatewayDialOption() grpc.DialOption {
	return grpc.WithConnectParams(grpc.ConnectParams{Backoff: backoff.DefaultConfig, MinConnectTimeout: gatewayConnectTimeout})
}
