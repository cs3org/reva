package runtime

import (
	"fmt"
	"path"
	"sort"

	"github.com/cs3org/reva/internal/http/interceptors/appctx"
	"github.com/cs3org/reva/internal/http/interceptors/auth"
	"github.com/cs3org/reva/internal/http/interceptors/log"
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
			return nil, fmt.Errorf("http middleware %s not found", name)
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
