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

package runtime

import (
	"sort"

	"github.com/cs3org/reva/internal/http/interceptors/appctx"
	"github.com/cs3org/reva/internal/http/interceptors/auth"
	"github.com/cs3org/reva/internal/http/interceptors/log"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/middlewares"
	"github.com/cs3org/reva/pkg/rhttp/mux"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// middlewareTriple represents a middleware with the
// priority to be chained.
type middlewareTriple struct {
	Name       string
	Priority   int
	Middleware rhttp.Middleware
}

func initHTTPMiddlewares(conf map[string]map[string]any, logger *zerolog.Logger) (func(*mux.Options) []middlewares.Middleware, error) {
	triples := []*middlewareTriple{}
	for name, c := range conf {
		new, ok := rhttp.NewMiddlewares[name]
		if !ok {
			continue
		}
		m, prio, err := new(c)
		if err != nil {
			return nil, errors.Wrapf(err, "error creating new middleware: %s,", name)
		}
		triples = append(triples, &middlewareTriple{
			Name:       name,
			Priority:   prio,
			Middleware: m,
		})
		logger.Info().Msgf("http middleware enabled: %s", name)
	}

	sort.SliceStable(triples, func(i, j int) bool {
		return triples[i].Priority > triples[j].Priority
	})

	authMiddle, err := auth.New(conf["auth"])
	if err != nil {
		return nil, errors.Wrap(err, "rhttp: error creating auth middleware")
	}
	logMiddle := log.New()
	appctxMiddle := appctx.New(*logger)

	return func(o *mux.Options) (m []middlewares.Middleware) {
		m = append(m, o.Middlewares...)
		for _, triple := range triples {
			m = append(m, middlewares.Middleware(triple.Middleware))
		}
		if !o.Unprotected {
			m = append(m, middlewares.Middleware(authMiddle))
		}
		m = append(m, logMiddle, appctxMiddle)
		return
	}, nil
}
