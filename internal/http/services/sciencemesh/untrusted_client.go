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
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/cs3org/reva/v3/internal/http/services/opencloudmesh/ocmd"
)

// overridePublicOnlyDialTimeout keeps sciencemesh's split timeouts after
// ocmd.NewPublicOnlyClient, which applies one duration to both the request
// and the dialer. The shared DialContext (public-only Control, scheme, and
// redirect policy) is left in place; only the dial deadline is tightened or
// replaced via context.
func overridePublicOnlyDialTimeout(c *ocmd.OCMClient, dialTimeout time.Duration) {
	if c == nil || dialTimeout <= 0 {
		return
	}
	tr := untrustedInnerTransport(c.HTTPTransport())
	if tr == nil || tr.DialContext == nil {
		return
	}
	inner := tr.DialContext
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()
		return inner(ctx, network, address)
	}
}

func untrustedInnerTransport(rt http.RoundTripper) *http.Transport {
	if tr, ok := rt.(*http.Transport); ok {
		return tr
	}
	v := reflect.ValueOf(rt)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("Transport")
	if !f.IsValid() || !f.CanInterface() {
		return nil
	}
	tr, ok := f.Interface().(*http.Transport)
	if !ok {
		return nil
	}
	return tr
}
