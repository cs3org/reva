package runtime

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/opencloud-eu/reva/v2/pkg/registry"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc"
	"github.com/opencloud-eu/reva/v2/pkg/rhttp"
	"github.com/rs/zerolog"
)

const (
	HTTP = iota
	GRPC
)

// RevaDrivenServer is an interface that defines the methods for starting and stopping reva HTTP/GRPC services.
type RevaDrivenServer interface {
	Start() error
	Stop() error
}

// NewDrivenHTTPServerWithOptions runs a revad server w/o watcher with the given config file and options.
// Use it in cases where you want to run a revad server without the need for a watcher and the os signal handling as a part of another runtime.
// Returns nil if no http server is configured in the config file.
// Throws a fatal error if the http server cannot be created.
func NewDrivenHTTPServerWithOptions(mainConf map[string]interface{}, opts ...Option) RevaDrivenServer {
	if !isEnabledHTTP(mainConf) {
		return nil
	}
	options := newOptions(opts...)
	if srv := newServer(HTTP, mainConf, options); srv != nil {
		return srv
	}
	options.Logger.Fatal().Msg("nothing to do, no http enabled_services declared in config")
	return nil
}

// NewDrivenGRPCServerWithOptions runs a revad server w/o watcher with the given config file and options.
// Use it in cases where you want to run a revad server without the need for a watcher and the os signal handling as a part of another runtime.
// Returns nil if no grpc server is configured in the config file.
// Throws a fatal error if the grpc server cannot be created.
func NewDrivenGRPCServerWithOptions(mainConf map[string]interface{}, opts ...Option) RevaDrivenServer {
	if !isEnabledGRPC(mainConf) {
		return nil
	}
	options := newOptions(opts...)
	if srv := newServer(GRPC, mainConf, options); srv != nil {
		return srv
	}
	options.Logger.Fatal().Msg("nothing to do, no grpc enabled_services declared in config")
	return nil
}

// drivenHTTPServer represents an HTTP server that implements the RevaDrivenServer interface.
type drivenHTTPServer struct {
	rhttpServer             *rhttp.Server
	log                     *zerolog.Logger
	gracefulShutdownTimeout int
}

// Start runs the revad HTTP drivenServer with the given config file.
func (s *drivenHTTPServer) Start() error {
	if s.rhttpServer == nil {
		err := fmt.Errorf("rhttp server not initialized")
		s.log.Fatal().Err(err).Send()
		return err
	}
	ln, err := net.Listen(s.rhttpServer.Network(), s.rhttpServer.Address())
	if err != nil {
		s.log.Fatal().Err(err).Send()
		return err
	}
	if err = s.rhttpServer.Start(ln); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("http server error")
		}
		return err
	}
	return nil
}

// Stop gracefully stops the revad drivenServer.
func (s *drivenHTTPServer) Stop() error {
	if s.rhttpServer == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		s.gracefulStopHTTPServer()
		close(done)
	}()

	select {
	case <-time.After(time.Duration(s.gracefulShutdownTimeout) * time.Second):
		s.log.Info().Msg("graceful shutdown timeout reached. running hard shutdown")
		s.stopHTTPServer()
		return nil
	case <-done:
		s.log.Info().Msg("all revad services gracefully stopped")
		return nil
	}
}

func (s *drivenHTTPServer) stopHTTPServer() {
	if s.rhttpServer == nil {
		return
	}
	s.log.Info().Msgf("fd to %s:%s abruptly closing", s.rhttpServer.Network(), s.rhttpServer.Address())
	err := s.rhttpServer.Stop()
	if err != nil {
		s.log.Error().Err(err).Msg("error stopping server")
	}
}

func (s *drivenHTTPServer) gracefulStopHTTPServer() {
	if s.rhttpServer != nil {
		s.log.Info().Msgf("fd to %s:%s gracefully closing", s.rhttpServer.Network(), s.rhttpServer.Address())
		if err := s.rhttpServer.GracefulStop(); err != nil {
			s.log.Error().Err(err).Msg("error gracefully stopping server")
			s.rhttpServer.Stop()
		}
	}
}

// drivenGRPCServer represents a GRPC server that implements the RevaDrivenServer interface.
type drivenGRPCServer struct {
	rgrpcServer             *rgrpc.Server
	log                     *zerolog.Logger
	gracefulShutdownTimeout int
}

// Start runs the revad GRPC drivenServer with the given config file.
func (s *drivenGRPCServer) Start() error {
	if s.rgrpcServer == nil {
		err := fmt.Errorf("rgrpc server not initialized")
		s.log.Fatal().Err(err).Send()
		return err
	}
	ln, err := net.Listen(s.rgrpcServer.Network(), s.rgrpcServer.Address())
	if err != nil {
		s.log.Fatal().Err(err).Send()
		return err
	}
	if err = s.rgrpcServer.Start(ln); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("grpc server error")
		}
		return err
	}
	return nil
}

// Stop gracefully stops the revad drivenServer.
func (s *drivenGRPCServer) Stop() error {
	if s.rgrpcServer == nil {
		return nil
	}

	done := make(chan struct{})
	go func() {
		s.gracefulStopGRPCServer()
		close(done)
	}()

	select {
	case <-time.After(time.Duration(s.gracefulShutdownTimeout) * time.Second):
		s.log.Info().Msg("graceful shutdown timeout reached. running hard shutdown")
		s.stopGRPCServer()
		return nil
	case <-done:
		s.log.Info().Msg("all revad services gracefully stopped")
		return nil
	}
}

func (s *drivenGRPCServer) gracefulStopGRPCServer() {
	if s.rgrpcServer != nil {
		s.log.Info().Msgf("fd to %s:%s gracefully closing", s.rgrpcServer.Network(), s.rgrpcServer.Address())
		if err := s.rgrpcServer.GracefulStop(); err != nil {
			s.log.Error().Err(err).Msg("error gracefully stopping server")
			s.rgrpcServer.Stop()
		}
	}
}

func (s *drivenGRPCServer) stopGRPCServer() {
	if s.rgrpcServer == nil {
		return
	}
	s.log.Info().Msgf("fd to %s:%s abruptly closing", s.rgrpcServer.Network(), s.rgrpcServer.Address())
	err := s.rgrpcServer.Stop()
	if err != nil {
		s.log.Error().Err(err).Msg("error stopping server")
	}
}

// newServer runs a revad server w/o watcher with the given config file and options.
func newServer(kind int, mainConf map[string]interface{}, options Options) RevaDrivenServer {
	parseSharedConfOrDie(mainConf["shared"])
	coreConf := parseCoreConfOrDie(mainConf["core"])
	log := options.Logger

	if err := registry.Init(options.Registry); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize registry client")
		return nil
	}

	host, _ := os.Hostname()
	log.Info().Msgf("host info: %s", host)

	// Only initialize tracing if we didn't get a tracer provider.
	if options.TraceProvider == nil {
		log.Debug().Msg("no pre-existing tracer given, initializing tracing")
		options.TraceProvider = initTracing(coreConf)
	}
	initCPUCount(coreConf, log)

	gracefulShutdownTimeout := 20
	if coreConf.GracefulShutdownTimeout > 0 {
		gracefulShutdownTimeout = coreConf.GracefulShutdownTimeout
	}
	switch kind {
	case HTTP:
		return initHTTPServer(mainConf, &options, gracefulShutdownTimeout)
	case GRPC:
		return initGRPCServer(mainConf, &options, gracefulShutdownTimeout)
	}
	return nil
}

func initHTTPServer(mainConf map[string]interface{}, options *Options, timeout int) RevaDrivenServer {
	s, err := getHTTPServer(mainConf["http"], options.Logger, options.TraceProvider)
	if err != nil {
		options.Logger.Fatal().Err(err).Msg("error creating http server")
		return nil
	}
	return &drivenHTTPServer{
		rhttpServer:             s,
		log:                     options.Logger,
		gracefulShutdownTimeout: timeout,
	}
}

func initGRPCServer(mainConf map[string]interface{}, options *Options, timeout int) RevaDrivenServer {
	s, err := getGRPCServer(mainConf["grpc"], options.Logger, options.TraceProvider)
	if err != nil {
		options.Logger.Fatal().Err(err).Msg("error creating grpc server")
		return nil
	}
	return &drivenGRPCServer{
		rgrpcServer:             s,
		gracefulShutdownTimeout: timeout,
		log:                     options.Logger,
	}
}
