package runtime

import (
	"net"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/rs/zerolog"
)

type Server struct {
	server   grace.Server
	listener net.Listener
	logger   *zerolog.Logger

	services map[string]any
}

func (s *Server) Start() error {
	return s.server.Start(s.listener)
}

func newServers(grpc map[string]*config.GRPC, http map[string]*config.HTTP) ([]*Server, error) {
	var servers []*Server
	for _, cfg := range grpc {
		services, err := rgrpc.InitServices(cfg.Services)
		if err != nil {
			return nil, err
		}
		s, err := rgrpc.NewServer(
			rgrpc.EnableReflection(cfg.EnableReflection),
			rgrpc.WithShutdownDeadline(cfg.ShutdownDeadline),
			rgrpc.WithLogger(zerolog.Nop()), // TODO: set logger
			rgrpc.WithServices(services),
		)
		if err != nil {
			return nil, err
		}
		server := &Server{
			server: s,
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
			rhttp.WithLogger(zerolog.Nop()), // TODO: set logger
			rhttp.WithCertAndKeyFiles(cfg.CertFile, cfg.KeyFile),
			// rhttp.WithMiddlewares(cfg.Middlewares),
		)
		if err != nil {
			return nil, err
		}
		server := &Server{
			server: s,
		}
		servers = append(servers, server)
	}
	return servers, nil
}
