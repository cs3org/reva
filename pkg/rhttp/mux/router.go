// Copyright 2018-2023 CERN
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

package mux

import (
	"context"
	"net/http"
)

type Params map[string]string

func (p Params) Get(key string) (string, bool) {
	if p == nil {
		return "", false
	}
	v, ok := p[key]
	return v, ok
}

// Router allows registering HTTP services.
type Router interface {
	http.Handler
	Walker

	// Route mounts a sub-Router along a path string.
	Route(path string, f func(Router), o ...Option)
	// Method routes for path that matches the HTTP method.
	Method(method, path string, handler http.Handler, o ...Option)

	// Handle routes for path that matches all the HTTP methods.
	Handle(path string, handler http.Handler, o ...Option)

	Mount(path string, handler http.Handler)

	With(path string, o ...Option)

	// HTTP-method routing along path.
	Get(path string, handler http.Handler, o ...Option)
	Head(path string, handler http.Handler, o ...Option)
	Post(path string, handler http.Handler, o ...Option)
	Put(path string, handler http.Handler, o ...Option)
	Patch(path string, handler http.Handler, o ...Option)
	Delete(path string, handler http.Handler, o ...Option)
	Connect(path string, handler http.Handler, o ...Option)
	Options(path string, handler http.Handler, o ...Option)
}

type WalkFunc func(method, path string, handler http.Handler, opts *Options)

type Walker interface {
	Walk(ctx context.Context, f WalkFunc)
}

func ParamsFromContext(ctx context.Context) Params {
	p, _ := ctx.Value(paramsKey{}).(Params)
	return p
}

func ParamsFromRequest(r *http.Request) Params {
	return ParamsFromContext(r.Context())
}
