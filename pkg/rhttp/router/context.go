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

package router

import "context"

type routeKey struct{}

// WithRoute returns a context carrying the route a request resolved to. The
// server resolves it once, before the middleware chain runs, so that a
// middleware decides on the route the request will actually reach rather than
// on a guess made from its path.
func WithRoute(ctx context.Context, r Route) context.Context {
	return context.WithValue(ctx, routeKey{}, r)
}

// RouteFromContext returns the route the request resolved to. It reports false
// when no route claimed the request, which a middleware must read as the
// stricter case: an unclaimed request is not an unprotected one.
func RouteFromContext(ctx context.Context) (Route, bool) {
	r, ok := ctx.Value(routeKey{}).(Route)
	return r, ok
}
