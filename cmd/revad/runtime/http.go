// Copyright 2018-2024 CERN
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
	"cmp"
	"context"
	"encoding/json"
	"sort"
	"sync"

	"github.com/cs3org/reva/v3/internal/http/interceptors/appctx"
	authmw "github.com/cs3org/reva/v3/internal/http/interceptors/auth"
	"github.com/cs3org/reva/v3/internal/http/interceptors/log"
	"github.com/cs3org/reva/v3/internal/http/interceptors/trace"
	"github.com/cs3org/reva/v3/pkg/auth"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/openapi"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rhttp/router"
	"github.com/cs3org/reva/v3/pkg/rversion"
	"github.com/getkin/kin-openapi/openapi3"
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

func initHTTPMiddlewares(conf map[string]map[string]any, logger *zerolog.Logger) ([]global.Middleware, error) {
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

	authMiddle, err := authmw.New(conf["auth"])
	if err != nil {
		return nil, errors.Wrap(err, "rhttp: error creating auth middleware")
	}

	middlewares := []global.Middleware{
		authMiddle,
		log.New(),
		appctx.New(*logger),
		trace.New(),
	}

	for _, triple := range triples {
		middlewares = append(middlewares, triple.Middleware)
	}

	return middlewares, nil
}

// withAuthSchemes carries how a server authenticates into the context its
// services are built with, and returns it.
//
// It is the same configuration the auth middleware reads, resolved a second
// time rather than restated: a service that publishes the server's API, such
// as openapi, then describes what the middleware actually accepts.
func withAuthSchemes(ctx context.Context, conf map[string]map[string]any) (context.Context, []auth.Scheme, error) {
	schemes, err := authmw.Schemes(conf["auth"])
	if err != nil {
		return nil, nil, errors.Wrap(err, "rhttp: error describing the authentication")
	}
	return auth.WithSchemes(ctx, schemes), schemes, nil
}

// describeAPIs gives each HTTP service instance the API invocation, so that
// the openapi service can describe a whole fleet. A document is built only
// when first asked for, because most processes are never asked, and the
// routes do not change after startup.
func describeAPIs(services map[string]global.Service, routes []router.Route, schemes []auth.Scheme, addr string) {
	for name := range services {
		var owned []router.Route
		for _, rt := range routes {
			if rt.Owner == name {
				owned = append(owned, rt)
			}
		}
		if len(owned) == 0 {
			continue
		}

		describe := sync.OnceValues(func() (invoke.Result, error) {
			doc, err := openapi.From(owned, &openapi3.Info{
				Title:   name,
				Version: cmp.Or(rversion.Version, "unknown"),
			}, schemes)
			if err != nil {
				return nil, err
			}
			encoded, err := json.Marshal(doc)
			if err != nil {
				return nil, err
			}
			return invoke.Result{"document": json.RawMessage(encoded)}, nil
		})
		invoke.DescribeAPI(nodeID(addr, name), func(context.Context) (invoke.Result, error) {
			return describe()
		})
	}
}
