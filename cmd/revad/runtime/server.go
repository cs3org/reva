package runtime

import (
	"net"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/utils/maps"
	"github.com/rs/zerolog"
)

type Server struct {
	server   grace.Server
	listener net.Listener

	services map[string]any
}

func (s *Server) Start() error {
	return s.server.Start(s.listener)
}

func newServers(grpc map[string]*config.GRPC, http map[string]*config.HTTP, log *zerolog.Logger) ([]*Server, error) {
	var servers []*Server
	for _, cfg := range grpc {
		services, err := rgrpc.InitServices(cfg.Services)
		if err != nil {
			return nil, err
		}
		s, err := rgrpc.NewServer(
			rgrpc.EnableReflection(cfg.EnableReflection),
			rgrpc.WithShutdownDeadline(cfg.ShutdownDeadline),
			rgrpc.WithLogger(log.With().Str("pkg", "grpc").Logger()),
			rgrpc.WithServices(services),
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
