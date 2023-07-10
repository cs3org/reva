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
	"path"
	"strings"
)

type Option int

const (
	Unprotected Option = iota
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
	Route(path string, f func(Router))
	// Handle routes for path that matches the HTTP method.
	Handle(method, path string, handler http.Handler)

	// HTTP-method routing along path.
	Get(path string, handler http.Handler)
	Head(path string, handler http.Handler)
	Post(path string, handler http.Handler)
	Put(path string, handler http.Handler)
	Delete(path string, handler http.Handler)
	Connect(path string, handler http.Handler)
	Options(path string, handler http.Handler)
}

type WalkFunc func(method, path string, handler http.Handler)

type Walker interface {
	Walk(ctx context.Context, f WalkFunc)
}

// ShiftPath splits off the first component of p, which will be cleaned of
// relative components before processing. head will never contain a slash and
// tail will always be a rooted path without trailing slash.
// see https://blog.merovius.de/2017/06/18/how-not-to-use-an-http-router.html
// and https://gist.github.com/weatherglass/62bd8a704d4dfdc608fe5c5cb5a6980c#gistcomment-2161690 for the zero alloc code below.
func ShiftPath(p string) (head, tail string) {
	if p == "" {
		return "", "/"
	}
	p = strings.TrimPrefix(path.Clean(p), "/")
	i := strings.Index(p, "/")
	if i < 0 {
		return p, "/"
	}
	return p[:i], p[i:]
}

func ParamsFromContext(ctx context.Context) Params {
	p, _ := ctx.Value(paramsKey{}).(Params)
	return p
}

func ParamsFromRequest(r *http.Request) Params {
	return ParamsFromContext(r.Context())
}
