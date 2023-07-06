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
	"net/http"
	"path"
	"strings"
)

type Option int

const (
	Unprotected Option = iota
)

type Params map[string]string

// Router allows registering HTTP services.
type Router interface {
	http.Handler
	Routes

	// Route mounts a sub-Router along a pattern string.
	Route(pattern string, f func(Router))
	// Handle routes for pattern that matches the HTTP method.
	Handle(method, pattern string, handler http.Handler)

	// HTTP-method routing along pattern.
	Get(pattern string, handler http.Handler)
	Head(pattern string, handler http.Handler)
	Post(pattern string, handler http.Handler)
	Put(pattern string, handler http.Handler)
	Delete(pattern string, handler http.Handler)
	Connect(pattern string, handler http.Handler)
	Options(pattern string, handler http.Handler)
}

// Routes makes the router traversable.
type Routes interface {
	// Routes gets the list of routes
	Routes() []Route
}

// Route holds the details of a single route.
type Route struct {
	Routes   Routes
	Pattern  string
	Handlers map[string]http.Handler // map[method]handler
	Options  []Option
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
