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

package rgrpc

import (
	"fmt"
	"io"
	"net"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// UnaryInterceptors is a map of registered unary grpc interceptors.
var UnaryInterceptors = map[string]NewUnaryInterceptor{}

// StreamInterceptors is a map of registered streaming grpc interceptor.
var StreamInterceptors = map[string]NewStreamInterceptor{}

// NewUnaryInterceptor is the type that unary interceptors need to register.
type NewUnaryInterceptor func(m map[string]interface{}) (grpc.UnaryServerInterceptor, int, error)

// NewStreamInterceptor is the type that stream interceptors need to register.
type NewStreamInterceptor func(m map[string]interface{}) (grpc.StreamServerInterceptor, int, error)

// RegisterUnaryInterceptor registers a new unary interceptor.
func RegisterUnaryInterceptor(name string, newFunc NewUnaryInterceptor) {
	UnaryInterceptors[name] = newFunc
}

// RegisterStreamInterceptor registers a new stream interceptor.
func RegisterStreamInterceptor(name string, newFunc NewStreamInterceptor) {
	StreamInterceptors[name] = newFunc
}

// Services is a map of service name and its new function.
var Services = map[string]NewService{}

// Register registers a new gRPC service with name and new function.
func Register(name string, newFunc NewService) {
	Services[name] = newFunc
}

// NewService is the function that gRPC services need to register at init time.
// It returns an io.Closer to close the service and a list of service endpoints that need to be unprotected.
type NewService func(conf map[string]interface{}) (Service, error)

// Service represents a grpc service.
type Service interface {
	Register(ss *grpc.Server)
	io.Closer
	UnprotectedEndpoints() []string
}

// Server is a gRPC server.
type Server struct {
	ShutdownDeadline         int
	EnableReflection         bool
	UnaryServerInterceptors  []grpc.UnaryServerInterceptor
	StreamServerInterceptors []grpc.StreamServerInterceptor

	s        *grpc.Server
	listener net.Listener
	log      zerolog.Logger
	services map[string]Service
}

func InitServices(services map[string]config.ServicesConfig) (map[string]Service, error) {
	s := make(map[string]Service)
	for name, cfg := range services {
		new, ok := Services[name]
		if !ok {
			return nil, fmt.Errorf("rgrpc: grpc service %s does not exist", name)
		}
		if cfg.DriversNumber() > 1 {
			return nil, fmt.Errorf("rgrp: service %s cannot have more than one driver in same server", name)
		}
		svc, err := new(cfg[0].Config)
		if err != nil {
			return nil, errors.Wrapf(err, "rgrpc: grpc service %s could not be started,", name)
		}
		s[name] = svc
	}
	return s, nil
}

// NewServer returns a new Server.
func NewServer(o ...Option) (*Server, error) {
	server := &Server{}
	for _, oo := range o {
		oo(server)
	}

	return server, nil
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	if err := s.initServices(); err != nil {
		err = errors.Wrap(err, "unable to register services")
		return err
	}

	s.listener = ln
	s.log.Info().Msgf("grpc server listening at %s:%s", s.Network(), s.Address())
	err := s.s.Serve(s.listener)
	if err != nil {
		err = errors.Wrap(err, "serve failed")
		return err
	}
	return nil
}

func (s *Server) initServices() error {
	opts := s.getInterceptors()
	grpcServer := grpc.NewServer(opts...)

	for _, svc := range s.services {
		svc.Register(grpcServer)
	}

	if s.EnableReflection {
		s.log.Info().Msg("rgrpc: grpc server reflection enabled")
		reflection.Register(grpcServer)
	}

	s.s = grpcServer

	return nil
}

// TODO(labkode): make closing with deadline.
func (s *Server) cleanupServices() {
	for name, svc := range s.services {
		if err := svc.Close(); err != nil {
			s.log.Error().Err(err).Msgf("error closing service %q", name)
		} else {
			s.log.Info().Msgf("service %q correctly closed", name)
		}
	}
}

// Stop stops the server.
func (s *Server) Stop() error {
	s.cleanupServices()
	s.s.Stop()
	return nil
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() error {
	s.cleanupServices()
	s.s.GracefulStop()
	return nil
}

// Network returns the network type.
func (s *Server) Network() string {
	return s.listener.Addr().Network()
}

// Address returns the network address.
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

func (s *Server) getInterceptors() []grpc.ServerOption {
	unaryChain := grpc_middleware.ChainUnaryServer(s.UnaryServerInterceptors...)
	streamChain := grpc_middleware.ChainStreamServer(s.StreamServerInterceptors...)

	return []grpc.ServerOption{
		grpc.UnaryInterceptor(unaryChain),
		grpc.StreamInterceptor(streamChain),
	}
}
