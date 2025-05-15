package runtime

import (
	"errors"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/opencloud-eu/reva/v2/pkg/registry"
	"github.com/opencloud-eu/reva/v2/pkg/rgrpc"
	"github.com/opencloud-eu/reva/v2/pkg/rhttp"
	"github.com/rs/zerolog"
)

type drivenServer struct {
	rhttpServer             *rhttp.Server
	rgrpcServer             *rgrpc.Server
	gracefulShutdownTimeout int
	pidFile                 string
	log                     *zerolog.Logger
}

// RevaDrivenServer is an interface that defines the methods for starting and stopping reva services.
type RevaDrivenServer interface {
	Start() error
	Stop() error
}

// RunDrivenServerWithOptions runs a revad server w/o watcher with the given config file, pid file and options.
// Use it in cases where you want to run a revad server without the need for a watcher and the os signal handling as a part of another runtime.
func RunDrivenServerWithOptions(mainConf map[string]interface{}, pidFile string, opts ...Option) RevaDrivenServer {
	options := newOptions(opts...)
	parseSharedConfOrDie(mainConf["shared"])
	coreConf := parseCoreConfOrDie(mainConf["core"])
	log := options.Logger

	if err := registry.Init(options.Registry); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize registry client")
	}

	host, _ := os.Hostname()
	log.Info().Msgf("host info: %s", host)

	// Only initialize tracing if we didn't get a tracer provider.
	if options.TraceProvider == nil {
		log.Debug().Msg("no pre-existing tracer given, initializing tracing")
		options.TraceProvider = initTracing(coreConf)
	}
	initCPUCount(coreConf, log)

	server := &drivenServer{
		rhttpServer: initHTTPServer(mainConf, &options),
		rgrpcServer: initGRPCServer(mainConf, &options),
		log:         log,
		pidFile:     pidFile,
	}
	server.gracefulShutdownTimeout = 30
	if coreConf.GracefulShutdownTimeout > 0 {
		server.gracefulShutdownTimeout = coreConf.GracefulShutdownTimeout
	}

	if server.rhttpServer == nil && server.rgrpcServer == nil {
		log.Fatal().Msg("nothing to do, no grpc/http enabled_services declared in config")
	}
	return server
}

// Start runs the revad drivenServer with the given config file and pid file.
func (s *drivenServer) Start() error {
	errCh := make(chan error, 2)
	done := make(chan struct{}, 1)
	wg := &sync.WaitGroup{}

	if s.rhttpServer != nil {
		wg.Add(1)
		go func() {
			errCh <- s.startHTTPServer()
			wg.Done()
		}()
	}
	if s.rgrpcServer != nil {
		wg.Add(1)
		go func() {
			errCh <- s.startGRPCServer()
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(done)
	}()
	for {
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
		case <-done:
			return nil
		}
	}
}

// Stop gracefully stops the revad drivenServer.
func (s *drivenServer) Stop() error {
	wg := &sync.WaitGroup{}

	s.gracefulStopHTTPServer(wg)
	s.gracefulStopGRPCServer(wg)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-time.After(time.Duration(s.gracefulShutdownTimeout) * time.Second):
		s.log.Info().Msg("graceful shutdown timeout reached. running hard shutdown")
		s.stopHTTPServer()
		s.stopGRPCServer()
		return nil
	case <-done:
		s.log.Info().Msg("all revad services gracefully stopped")
		return nil
	}
}

func (s *drivenServer) startHTTPServer() error {
	if s.rhttpServer == nil {
		s.log.Fatal().Msg("http server not initialized")
	}
	ln, err := net.Listen(s.rhttpServer.Network(), s.rhttpServer.Address())
	if err != nil {
		s.log.Fatal().Err(err).Send()
	}
	if err = s.rhttpServer.Start(ln); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("http server error")
		}
		return err
	}
	return nil
}

func (s *drivenServer) startGRPCServer() error {
	if s.rgrpcServer == nil {
		s.log.Fatal().Msg("grcp server not initialized")
	}
	ln, err := net.Listen(s.rgrpcServer.Network(), s.rgrpcServer.Address())
	if err != nil {
		s.log.Fatal().Err(err).Send()
	}
	if err = s.rgrpcServer.Start(ln); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("grpc server error")
		}
		return err
	}
	return nil
}

func (s *drivenServer) gracefulStopHTTPServer(wg *sync.WaitGroup) {
	if s.rhttpServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.log.Info().Msgf("fd to %s:%s gracefully closing", s.rhttpServer.Network(), s.rhttpServer.Address())
			if err := s.rhttpServer.GracefulStop(); err != nil {
				s.log.Error().Err(err).Msg("error gracefully stopping server")
				s.rhttpServer.Stop()
			}
		}()
	}
}

func (s *drivenServer) gracefulStopGRPCServer(wg *sync.WaitGroup) {
	if s.rgrpcServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.log.Info().Msgf("fd to %s:%s gracefully closing", s.rgrpcServer.Network(), s.rgrpcServer.Address())
			if err := s.rgrpcServer.GracefulStop(); err != nil {
				s.log.Error().Err(err).Msg("error gracefully stopping server")
				s.rgrpcServer.Stop()
			}
		}()
	}
}

func (s *drivenServer) stopHTTPServer() {
	if s.rhttpServer == nil {
		return
	}
	s.log.Info().Msgf("fd to %s:%s abruptly closing", s.rhttpServer.Network(), s.rhttpServer.Address())
	err := s.rhttpServer.Stop()
	if err != nil {
		s.log.Error().Err(err).Msg("error stopping server")
	}
}

func (s *drivenServer) stopGRPCServer() {
	if s.rgrpcServer == nil {
		return
	}
	s.log.Info().Msgf("fd to %s:%s abruptly closing", s.rgrpcServer.Network(), s.rgrpcServer.Address())
	err := s.rgrpcServer.Stop()
	if err != nil {
		s.log.Error().Err(err).Msg("error stopping server")
	}
}

func initHTTPServer(mainConf map[string]interface{}, options *Options) *rhttp.Server {
	if isEnabledHTTP(mainConf) {
		s, err := getHTTPServer(mainConf["http"], options.Logger, options.TraceProvider)
		if err != nil {
			options.Logger.Fatal().Err(err).Msg("error creating http server")
		}
		return s
	}
	return nil
}

func initGRPCServer(mainConf map[string]interface{}, options *Options) *rgrpc.Server {
	if isEnabledGRPC(mainConf) {
		s, err := getGRPCServer(mainConf["grpc"], options.Logger, options.TraceProvider)
		if err != nil {
			options.Logger.Fatal().Err(err).Msg("error creating grpc server")
		}
		return s
	}
	return nil
}
