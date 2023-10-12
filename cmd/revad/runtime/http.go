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
	"path"
	"sort"

	"github.com/cs3org/reva/internal/http/interceptors/appctx"
	"github.com/cs3org/reva/internal/http/interceptors/auth"
	"github.com/cs3org/reva/internal/http/interceptors/log"
	"github.com/cs3org/reva/internal/http/interceptors/metrics"
	"github.com/cs3org/reva/internal/http/interceptors/trace"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// middlewareTriple represents a middleware with the
// priority to be chained.
type middlewareTriple struct {
	Name       string
	Priority   int
	Middleware global.Middleware
}

func initHTTPMiddlewares(conf map[string]map[string]any, unprotected []string, logger *zerolog.Logger) ([]global.Middleware, error) {
	triples := []*middlewareTriple{}
	for name, c := range conf {
		new, ok := global.NewMiddlewares[name]
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

	authMiddle, err := auth.New(conf["auth"], unprotected)
	if err != nil {
		return nil, errors.Wrap(err, "rhttp: error creating auth middleware")
	}

	middlewares := []global.Middleware{
		authMiddle,
		log.New(),
		appctx.New(*logger),
		metrics.New(),
		trace.New(),
	}

	for _, triple := range triples {
		middlewares = append(middlewares, triple.Middleware)
	}
	return middlewares, nil
}

func httpUnprotected(s map[string]global.Service) (unprotected []string) {
	for _, svc := range s {
		for _, url := range svc.Unprotected() {
			unprotected = append(unprotected, path.Join("/", svc.Prefix(), url))
		}
	}
	return
}
