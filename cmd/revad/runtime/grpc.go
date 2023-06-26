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
	"fmt"
	"sort"

	"github.com/cs3org/reva/internal/grpc/interceptors/appctx"
	"github.com/cs3org/reva/internal/grpc/interceptors/auth"
	"github.com/cs3org/reva/internal/grpc/interceptors/log"
	"github.com/cs3org/reva/internal/grpc/interceptors/recovery"
	"github.com/cs3org/reva/internal/grpc/interceptors/token"
	"github.com/cs3org/reva/internal/grpc/interceptors/useragent"
	"github.com/cs3org/reva/pkg/rgrpc"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

type unaryInterceptorTriple struct {
	Name        string
	Priority    int
	Interceptor grpc.UnaryServerInterceptor
}

type streamInterceptorTriple struct {
	Name        string
	Priority    int
	Interceptor grpc.StreamServerInterceptor
}

func initGRPCInterceptors(conf map[string]map[string]any, unprotected []string, logger *zerolog.Logger) ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor, error) {
	unaryTriples := []*unaryInterceptorTriple{}
	for name, c := range conf {
		new, ok := rgrpc.UnaryInterceptors[name]
		if !ok {
			return nil, nil, fmt.Errorf("unary interceptor %s not found", name)
		}
		inter, prio, err := new(c)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error creating unary interceptor: "+name)
		}
		triple := &unaryInterceptorTriple{
			Name:        name,
			Priority:    prio,
			Interceptor: inter,
		}
		unaryTriples = append(unaryTriples, triple)
	}

	sort.SliceStable(unaryTriples, func(i, j int) bool {
		return unaryTriples[i].Priority < unaryTriples[j].Priority
	})

	authUnary, err := auth.NewUnary(conf["auth"], unprotected)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error creating unary auth interceptor")
	}

	unaryInterceptors := []grpc.UnaryServerInterceptor{authUnary}
	for _, t := range unaryTriples {
		unaryInterceptors = append(unaryInterceptors, t.Interceptor)
		logger.Info().Msgf("rgrpc: chaining grpc unary interceptor %s with priority %d", t.Name, t.Priority)
	}

	unaryInterceptors = append(unaryInterceptors,
		otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(rtrace.Provider),
			otelgrpc.WithPropagators(rtrace.Propagator)),
	)

	unaryInterceptors = append([]grpc.UnaryServerInterceptor{
		appctx.NewUnary(*logger),
		token.NewUnary(),
		useragent.NewUnary(),
		log.NewUnary(),
		recovery.NewUnary(),
	}, unaryInterceptors...)

	streamTriples := []*streamInterceptorTriple{}
	for name, c := range conf {
		new, ok := rgrpc.StreamInterceptors[name]
		if !ok {
			return nil, nil, fmt.Errorf("stream interceptor %s not found", name)
		}
		inter, prio, err := new(c)
		if err != nil {
			if err != nil {
				return nil, nil, errors.Wrapf(err, "error creating streaming interceptor: %s,", name)
			}
			triple := &streamInterceptorTriple{
				Name:        name,
				Priority:    prio,
				Interceptor: inter,
			}
			streamTriples = append(streamTriples, triple)
		}

	}
	// sort stream triples
	sort.SliceStable(streamTriples, func(i, j int) bool {
		return streamTriples[i].Priority < streamTriples[j].Priority
	})

	authStream, err := auth.NewStream(conf["auth"], unprotected)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error creating stream auth interceptor")
	}

	streamInterceptors := []grpc.StreamServerInterceptor{authStream}
	for _, t := range streamTriples {
		streamInterceptors = append(streamInterceptors, t.Interceptor)
		logger.Info().Msgf("rgrpc: chaining grpc streaming interceptor %s with priority %d", t.Name, t.Priority)
	}

	streamInterceptors = append([]grpc.StreamServerInterceptor{
		authStream,
		appctx.NewStream(*logger),
		token.NewStream(),
		useragent.NewStream(),
		log.NewStream(),
		recovery.NewStream(),
	}, streamInterceptors...)

	return unaryInterceptors, streamInterceptors, nil
}

func grpcUnprotected(s map[string]rgrpc.Service) (unprotected []string) {
	for _, svc := range s {
		unprotected = append(unprotected, svc.UnprotectedEndpoints()...)
	}
	return
}
