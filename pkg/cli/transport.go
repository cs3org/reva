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
	"time"

	"github.com/cs3org/reva/v3/pkg/cli/backend"
	grpcbackend "github.com/cs3org/reva/v3/pkg/cli/backend/grpc"
	"github.com/cs3org/reva/v3/pkg/cli/data"
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

var errHTTPNotBuilt = errors.New("the http transport is not available yet in this version of the CLI: use --transport grpc with --gateway")

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
	case t == backend.HTTP:
		return nil, errHTTPNotBuilt
	case t == TransportAuto:
		return nil, fmt.Errorf("no gateway is set, and the http transport is not available yet in this version of the CLI: pass --gateway%s", r.envHint("gateway"))
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
	return nil, errHTTPNotBuilt
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
	r.data = data.New(r.HTTPClient().GetNativeHTTP(), b.Locator())
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
