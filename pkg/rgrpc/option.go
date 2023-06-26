package rgrpc

import (
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

type Option func(*Server)

func WithShutdownDeadline(deadline int) Option {
	return func(s *Server) {
		s.ShutdownDeadline = deadline
	}
}

func EnableReflection(enable bool) Option {
	return func(s *Server) {
		s.EnableReflection = enable
	}
}

func WithServices(services map[string]Service) Option {
	return func(s *Server) {
		s.services = services
	}
}

func WithLogger(logger zerolog.Logger) Option {
	return func(s *Server) {
		s.log = logger
	}
}

func WithStreamServerInterceptors(in []grpc.StreamServerInterceptor) Option {
	return func(s *Server) {
		s.StreamServerInterceptors = in
	}
}

func WithUnaryServerInterceptors(in []grpc.UnaryServerInterceptor) Option {
	return func(s *Server) {
		s.UnaryServerInterceptors = in
	}
}