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

package auth

import "context"

// SchemeKind is where on the wire a credential goes.
type SchemeKind string

const (
	// SchemeBasic is HTTP basic authentication.
	SchemeBasic SchemeKind = "basic"
	// SchemeBearer is a token in the Authorization header, "Bearer" prefixed.
	SchemeBearer SchemeKind = "bearer"
	// SchemeNegotiate is a SPNEGO token in the Authorization header, as RFC
	// 4559 negotiates it.
	SchemeNegotiate SchemeKind = "negotiate"
	// SchemeHeader is a token in a header of its own.
	SchemeHeader SchemeKind = "header"
	// SchemeQuery is a token in a query parameter.
	SchemeQuery SchemeKind = "query"
)

// Scheme is one way of authenticating that a server accepts. A server is
// configured with chains of strategies, which is how it reads a request; a
// Scheme is that same arrangement turned around, to tell a client what to
// send.
type Scheme struct {
	// Name identifies the scheme to a client, after the strategy that accepts
	// it.
	Name string
	// Kind is where the credential goes.
	Kind SchemeKind
	// Param names the header or query parameter, for the kinds that use one.
	Param string
	// Description is addressed to whoever has to authenticate.
	Description string
}

type schemesKey struct{}

// WithSchemes carries the schemes an HTTP server accepts. A service that
// describes that server's API reads them from its context instead of being
// configured with them a second time, so what a deployment publishes cannot
// disagree with what it accepts.
func WithSchemes(ctx context.Context, schemes []Scheme) context.Context {
	return context.WithValue(ctx, schemesKey{}, schemes)
}

// SchemesFromContext returns the schemes WithSchemes carried, and nil if
// nothing did.
func SchemesFromContext(ctx context.Context) []Scheme {
	schemes, _ := ctx.Value(schemesKey{}).([]Scheme)
	return schemes
}
