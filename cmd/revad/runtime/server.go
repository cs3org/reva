package runtime

import (
	"fmt"
	"net"
	"sort"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/internal/grpc/interceptors/appctx"
	"github.com/cs3org/reva/internal/grpc/interceptors/auth"
	"github.com/cs3org/reva/internal/grpc/interceptors/log"
	"github.com/cs3org/reva/internal/grpc/interceptors/recovery"
	"github.com/cs3org/reva/internal/grpc/interceptors/token"
	"github.com/cs3org/reva/internal/grpc/interceptors/useragent"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils/maps"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
)

type Server struct {
	server   grace.Server
	listener net.Listener

	services map[string]any
}

func (s *Server) Start() error {
	return s.server.Start(s.listener)
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

func newServers(grpc map[string]*config.GRPC, http map[string]*config.HTTP, log *zerolog.Logger) ([]*Server, error) {
	var servers []*Server
	for _, cfg := range grpc {
		services, err := rgrpc.InitServices(cfg.Services)
		if err != nil {
			return nil, err
		}
		unaryChain, streamChain, err := initGRPCInterceptors(cfg.Interceptors, grpcUnprotected(services), log)
		if err != nil {
			return nil, err
		}
		s, err := rgrpc.NewServer(
			rgrpc.EnableReflection(cfg.EnableReflection),
			rgrpc.WithShutdownDeadline(cfg.ShutdownDeadline),
			rgrpc.WithLogger(log.With().Str("pkg", "grpc").Logger()),
			rgrpc.WithServices(services),
			rgrpc.WithUnaryServerInterceptors(unaryChain),
			rgrpc.WithStreamServerInterceptors(streamChain),
		)
		if err != nil {
			return nil, err
		}
		server := &Server{
			server:   s,
			services: maps.MapValues(services, func(s rgrpc.Service) any { return s }),
		}
		servers = append(servers, server)
	}
	for _, cfg := range http {
		services, err := rhttp.InitServices(cfg.Services)
		if err != nil {
			return nil, err
		}
		s, err := rhttp.New(
			rhttp.WithServices(services),
			rhttp.WithLogger(log.With().Str("pkg", "http").Logger()),
			rhttp.WithCertAndKeyFiles(cfg.CertFile, cfg.KeyFile),
			// rhttp.WithMiddlewares(cfg.Middlewares),
		)
		if err != nil {
			return nil, err
		}
		server := &Server{
			server:   s,
			services: maps.MapValues(services, func(s global.Service) any { return s }),
		}
		servers = append(servers, server)
	}
	return servers, nil
}

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
