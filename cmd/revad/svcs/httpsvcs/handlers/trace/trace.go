// Copyright 2018-2019 CERN
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

package trace

import (
	"fmt"
	"net/http"

	"github.com/cernbox/reva/cmd/revad/httpserver"
	tracepkg "github.com/cernbox/reva/pkg/trace"
	"github.com/gofrs/uuid"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc/metadata"
)

type config struct {
	Priority int    `mapstructure:"priority"`
	Header   string `mapstructure:"header"`
}

func init() {
	httpserver.RegisterMiddleware("trace", New)
}

// New returns a middleware that checks if there is a trace provided
// as X-Trace header or generates one on the fly
// then the trace is stored in the context.
func New(m map[string]interface{}) (httpserver.Middleware, int, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		return nil, 0, err
	}

	if c.Header == "" {
		return nil, 0, fmt.Errorf("trace middleware: header trace is empty")
	}

	chain := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			var trace string
			val, ok := tracepkg.ContextGetTrace(ctx)
			if ok && val != "" {
				trace = val
			} else {
				// try to get it from header
				trace = r.Header.Get(c.Header)
				if trace == "" {
					trace = genTrace()
				}
			}

			ctx = tracepkg.ContextSetTrace(ctx, trace)
			ctx = metadata.AppendToOutgoingContext(ctx, c.Header, trace)
			fmt.Println(trace)
			r = r.WithContext(ctx)
			h.ServeHTTP(w, r)
		})
	}
	return chain, c.Priority, nil
}

func genTrace() string {
	return uuid.Must(uuid.NewV4()).String()
}
