package grpcserver

import (
	"context"
	"net"
	"sort"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
)

var (
	ctx    = context.Background()
	logger = log.New("grpcserver")
	errors = err.New("grpcserver")
)

// UnaryInterceptors is a map of registered unary grpc interceptors.
var UnaryInterceptors = map[string]NewUnaryInterceptor{}

// StreamInterceptors is a map of registered streaming grpc interceptor
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
type NewService func(conf map[string]interface{}, ss *grpc.Server) error

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

type config struct {
	Network             string                            `mapstructure:"network"`
	Address             string                            `mapstructure:"address"`
	ShutdownDeadline    int                               `mapstructure:"shutdown_deadline"`
	EnabledServices     []string                          `mapstructure:"enabled_services"`
	Services            map[string]map[string]interface{} `mapstructure:"services"`
	EnabledInterceptors []string                          `mapstructure:"enabled_interceptors"`
	Interceptors        map[string]map[string]interface{} `mapstructure:"interceptors"`
}

// Server is a gRPC server.
type Server struct {
	s        *grpc.Server
	conf     *config
	listener net.Listener
}

// New returns a new Server.
func New(m map[string]interface{}) (*Server, error) {
	conf := &config{}
	if err := mapstructure.Decode(m, conf); err != nil {
		return nil, err
	}

	// apply defaults
	if conf.Network == "" {
		conf.Network = "tcp"
	}

	if conf.Address == "" {
		conf.Address = "0.0.0.0:9999"
	}

	server := &Server{conf: conf}
	opts, err := server.getInterceptors()
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(opts...)
	server.s = grpcServer
	return server, nil
}

// Start starts the server.
func (s *Server) Start(ln net.Listener) error {
	if err := s.registerServices(); err != nil {
		err = errors.Wrap(err, "unable to register services")
		return err
	}

	s.listener = ln
	logger.Printf(ctx, "grpc server listening at %s:%s", s.Network(), s.Address())
	err := s.s.Serve(s.listener)
	if err != nil {
		err = errors.Wrap(err, "serve failed")
		return err
	}
	return nil
}

func (s *Server) isInterceptorEnabled(name string) bool {
	for _, k := range s.conf.EnabledInterceptors {
		if k == name {
			return true
		}
	}
	return false
}

func (s *Server) isServiceEnabled(name string) bool {
	for _, k := range s.conf.EnabledServices {
		if k == name {
			return true
		}
	}
	return false
}

func (s *Server) registerServices() error {
	for name, newFunc := range Services {
		if s.isServiceEnabled(name) {
			if err := newFunc(s.conf.Services[name], s.s); err != nil {
				return err
			}
			logger.Println(ctx, "grpc service enabled: "+name)
		}
	}
	return nil
}

// Stop stops the server.
func (s *Server) Stop() error {
	s.s.Stop()
	return nil
}

// GracefulStop gracefully stops the server.
func (s *Server) GracefulStop() error {
	s.s.GracefulStop()
	return nil
}

// Network returns the network type.
func (s *Server) Network() string {
	return s.conf.Network
}

// Address returns the network address.
func (s *Server) Address() string {
	return s.conf.Address
}

func (s *Server) getInterceptors() ([]grpc.ServerOption, error) {
	unaryTriples := []*unaryInterceptorTriple{}
	for name, newFunc := range UnaryInterceptors {
		if s.isInterceptorEnabled(name) {
			inter, prio, err := newFunc(s.conf.Interceptors[name])
			if err != nil {
				return nil, err
			}
			triple := &unaryInterceptorTriple{
				Name:        name,
				Priority:    prio,
				Interceptor: inter,
			}
			unaryTriples = append(unaryTriples, triple)
		}
	}

	// sort unary triples
	sort.SliceStable(unaryTriples, func(i, j int) bool {
		return unaryTriples[i].Priority < unaryTriples[j].Priority
	})

	unaryInterceptors := []grpc.UnaryServerInterceptor{}
	for _, t := range unaryTriples {
		unaryInterceptors = append(unaryInterceptors, t.Interceptor)
		logger.Printf(ctx, "chainning grpc unary interceptor %s with priority %d", t.Name, t.Priority)
	}
	unaryChain := grpc_middleware.ChainUnaryServer(unaryInterceptors...)

	streamTriples := []*streamInterceptorTriple{}
	for name, newFunc := range StreamInterceptors {
		if s.isInterceptorEnabled(name) {
			inter, prio, err := newFunc(s.conf.Interceptors[name])
			if err != nil {
				return nil, err
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

	streamInterceptors := []grpc.StreamServerInterceptor{}
	for _, t := range streamTriples {
		streamInterceptors = append(streamInterceptors, t.Interceptor)
		logger.Printf(ctx, "chainning grpc streaming interceptor %s with priority %d", t.Name, t.Priority)
	}
	streamChain := grpc_middleware.ChainStreamServer(streamInterceptors...)

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(unaryChain),
		grpc.StreamInterceptor(streamChain),
	}

	return opts, nil
}
