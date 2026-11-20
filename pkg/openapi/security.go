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

import "github.com/cs3org/reva/v3/pkg/auth"

// securitySchemes describes each scheme the way OpenAPI names it.
func securitySchemes(schemes []auth.Scheme) map[string]*SecurityScheme {
	if len(schemes) == 0 {
		return nil
	}
	out := make(map[string]*SecurityScheme, len(schemes))
	for _, s := range schemes {
		if scheme := securityScheme(s); scheme != nil {
			out[s.Name] = scheme
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func securityScheme(s auth.Scheme) *SecurityScheme {
	switch s.Kind {
	case auth.SchemeBasic:
		return &SecurityScheme{Type: "http", Scheme: "basic", Description: s.Description}
	case auth.SchemeBearer:
		return &SecurityScheme{Type: "http", Scheme: "bearer", Description: s.Description}
	case auth.SchemeNegotiate:
		// Registered with IANA, so it names an http scheme like the others.
		return &SecurityScheme{Type: "http", Scheme: "negotiate", Description: s.Description}
	case auth.SchemeHeader:
		return &SecurityScheme{Type: "apiKey", In: "header", Name: s.Param, Description: s.Description}
	case auth.SchemeQuery:
		return &SecurityScheme{Type: "apiKey", In: "query", Name: s.Param, Description: s.Description}
	}
	return nil
}

// requirement is what a protected operation asks for: any one of the schemes,
// in the order the deployment tries them. Every protected operation is given
// the same one, which the document is built and written without changing.
func requirement(schemes []auth.Scheme) []map[string][]string {
	var req []map[string][]string
	for _, s := range schemes {
		if securityScheme(s) == nil {
			continue
		}
		req = append(req, map[string][]string{s.Name: {}})
	}
	return req
}
