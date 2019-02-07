package grpcserver

import (
	"context"
	"fmt"
	"net"

	appproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/appprovider/v0alpha"
	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/appprovidersvc"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/appregistrysvc"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/authsvc"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/interceptors"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/storagebrokersvc"
	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/storageprovidersvc"
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
		err = errors.Wrap(err, "unable to register service")
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

func (s *Server) registerServices() error {
	enabled := []string{}
	for _, k := range s.conf.EnabledServices {
		switch k {
		case "storageprovidersvc":
			svc, err := storageprovidersvc.New(s.conf.Services[k])
			if err != nil {
				return errors.Wrap(err, "unable to register service "+k)
			}
			storageproviderv0alphapb.RegisterStorageProviderServiceServer(s.s, svc)
			enabled = append(enabled, k)
		case "authsvc":
			svc, err := authsvc.New(s.conf.Services[k])
			if err != nil {
				return errors.Wrap(err, "unable to register service "+k)
			}
			authv0alphapb.RegisterAuthServiceServer(s.s, svc)
			enabled = append(enabled, k)

		case "storagebrokersvc":
			svc, err := storagebrokersvc.New(s.conf.Services[k])
			if err != nil {
				return errors.Wrap(err, "unable to register service "+k)
			}
			storagebrokerv0alphapb.RegisterStorageBrokerServiceServer(s.s, svc)
			enabled = append(enabled, k)
		case "appregistrysvc":
			svc, err := appregistrysvc.New(s.conf.Services[k])
			if err != nil {
				return errors.Wrap(err, "unable to register service "+k)
			}
			appregistryv0alphapb.RegisterAppRegistryServiceServer(s.s, svc)
			enabled = append(enabled, k)
		case "appprovidersvc":
			svc, err := appprovidersvc.New(s.conf.Services[k])
			if err != nil {
				return errors.Wrap(err, "unable to register service "+k)
			}
			appproviderv0alphapb.RegisterAppProviderServiceServer(s.s, svc)
			enabled = append(enabled, k)
		}
	}
	if len(enabled) == 0 {
		logger.Println(ctx, "no grpc services enabled")
	} else {
		for k := range enabled {
			logger.Printf(ctx, "grpc service enabled: %s", enabled[k])
		}
	}
	return nil
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
