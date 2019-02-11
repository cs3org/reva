package grpcserver

import (
	"context"
	"fmt"
	"net"

	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors"
	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/mitchellh/mapstructure"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	ctx    = context.Background()
	logger = log.New("grpcsvr")
	errors = err.New("grpcsvr")
)

// Services is a map of service name and its new function.
var Services = map[string]NewService{}

// Register registers a new gRPC service with name and new function.
func Register(name string, newFunc NewService) {
	Services[name] = newFunc
}

// NewService is the function that gRPC services need to register at init time.
type NewService func(conf map[string]interface{}, ss *grpc.Server) error

type config struct {
	Network          string                            `mapstructure:"network"`
	Address          string                            `mapstructure:"address"`
	ShutdownDeadline int                               `mapstructure:"shutdown_deadline"`
	EnabledServices  []string                          `mapstructure:"enabled_services"`
	Services         map[string]map[string]interface{} `mapstructure:"services"`
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

	opts := getOpts()
	s := grpc.NewServer(opts...)

	return &Server{s: s, conf: conf}, nil
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

func getOpts() []grpc.ServerOption {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(
			grpc_middleware.ChainUnaryServer(
				grpc_recovery.UnaryServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc)),
				interceptors.TraceUnaryServerInterceptor(),
				interceptors.LogUnaryServerInterceptor(),
				grpc_prometheus.UnaryServerInterceptor)),
		grpc.StreamInterceptor(
			grpc_middleware.ChainStreamServer(
				grpc_recovery.StreamServerInterceptor(grpc_recovery.WithRecoveryHandlerContext(recoveryFunc)),
				interceptors.TraceStreamServerInterceptor(),
				grpc_prometheus.StreamServerInterceptor)),
	}
	return opts
}

func recoveryFunc(ctx context.Context, p interface{}) (err error) {
	logger.Panic(ctx, fmt.Sprintf("%+v", p))
	return grpc.Errorf(codes.Internal, "%s", p)
}
