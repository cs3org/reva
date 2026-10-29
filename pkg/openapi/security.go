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

package openapi

import (
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/getkin/kin-openapi/openapi3"
)

// securitySchemes describes each scheme the way OpenAPI names it.
func securitySchemes(schemes []auth.Scheme) openapi3.SecuritySchemes {
	out := openapi3.SecuritySchemes{}
	for _, s := range schemes {
		if scheme := securityScheme(s); scheme != nil {
			out[s.Name] = &openapi3.SecuritySchemeRef{Value: scheme}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func securityScheme(s auth.Scheme) *openapi3.SecurityScheme {
	scheme := openapi3.NewSecurityScheme().WithDescription(s.Description)
	switch s.Kind {
	case auth.SchemeBasic:
		return scheme.WithType("http").WithScheme("basic")
	case auth.SchemeBearer:
		return scheme.WithType("http").WithScheme("bearer")
	case auth.SchemeNegotiate:
		// Registered with IANA, so it names an http scheme like the others.
		return scheme.WithType("http").WithScheme("negotiate")
	case auth.SchemeHeader:
		return scheme.WithType("apiKey").WithIn("header").WithName(s.Param)
	case auth.SchemeQuery:
		return scheme.WithType("apiKey").WithIn("query").WithName(s.Param)
	}
	return nil
}

// requirement is what a protected operation asks for: any one of the schemes,
// in the order the deployment tries them. Every protected operation is given
// the same one, which the document is built and written without changing.
func requirement(schemes []auth.Scheme) *openapi3.SecurityRequirements {
	req := openapi3.NewSecurityRequirements()
	for _, s := range schemes {
		if securityScheme(s) == nil {
			continue
		}
		req.With(openapi3.NewSecurityRequirement().Authenticate(s.Name))
	}
	if len(*req) == 0 {
		return nil
	}
	return req
}
