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
