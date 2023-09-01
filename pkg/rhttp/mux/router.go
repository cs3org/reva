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

type Param struct {
	Key, Value string
}

type Params []Param

func (ps Params) Get(key string) (string, bool) {
	for _, p := range ps {
		if p.Key == key {
			return p.Value, true
		}
	}
	return "", false
}

type Handler interface {
	ServeHTTP(http.ResponseWriter, *http.Request, Params)
}

type HandlerFunc func(http.ResponseWriter, *http.Request, Params)

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request, p Params) {
	f(w, r, p)
}

type Middleware func(Handler) Handler

// Router allows registering HTTP services.
type Router interface {
	Walker

	// Route mounts a sub-Router along a path string.
	Route(path string, f func(Router))
	// Method routes for path that matches the HTTP method.
	Method(method, path string, handler Handler)

	// Handle routes for path that matches all the HTTP methods.
	Handle(path string, handler Handler)

	Mount(path string, handler Handler)

	Use(middlewares ...Middleware)

	// HTTP-method routing along path.
	Get(path string, handler Handler)
	Head(path string, handler Handler)
	Post(path string, handler Handler)
	Put(path string, handler Handler)
	Patch(path string, handler Handler)
	Delete(path string, handler Handler)
	Connect(path string, handler Handler)
	Options(path string, handler Handler)
}

type WalkFunc func(method, path string, handler Handler)

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
