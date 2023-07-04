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

package runtime

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rhttp"

	"github.com/cs3org/reva/pkg/rhttp/global"
	"github.com/cs3org/reva/pkg/rserverless"
	"github.com/cs3org/reva/pkg/sharedconf"
	rtrace "github.com/cs3org/reva/pkg/trace"
	"github.com/cs3org/reva/pkg/utils/list"
	"github.com/cs3org/reva/pkg/utils/maps"
	netutil "github.com/cs3org/reva/pkg/utils/net"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

// Reva represents a full instance of reva.
type Reva struct {
	ctx    context.Context
	config *config.Config

	servers    []*Server
	serverless *rserverless.Serverless
	watcher    *grace.Watcher
	lns        map[string]net.Listener

	pidfile string
	log     *zerolog.Logger
}

// Server represents a reva server (grpc or http).
type Server struct {
	server   grace.Server
	listener net.Listener

	services map[string]any
}

// Start starts the server listening on the assigned listener.
func (s *Server) Start() error {
	return s.server.Start(s.listener)
}

// New creates a new reva instance.
func New(config *config.Config, opt ...Option) (*Reva, error) {
	opts := newOptions(opt...)
	log := opts.Logger

	ctx := appctx.WithLogger(opts.Ctx, log)

	if err := initCPUCount(config.Core, log); err != nil {
		return nil, err
	}
	initTracing(config.Core)

	if opts.PidFile == "" {
		return nil, errors.New("pid file not provided")
	}

	watcher, err := initWatcher(opts.PidFile, log)
	if err != nil {
		return nil, err
	}

	listeners, err := watcher.GetListeners(servicesAddresses(config))
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	setRandomAddresses(config, listeners, log)

	if err := applyTemplates(config); err != nil {
		watcher.Clean()
		return nil, err
	}
	initSharedConf(config)

	grpc := groupGRPCByAddress(config)
	http := groupHTTPByAddress(config)
	servers, err := newServers(ctx, grpc, http, listeners, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	serverless, err := newServerless(config, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	return &Reva{
		ctx:        ctx,
		config:     config,
		servers:    servers,
		serverless: serverless,
		watcher:    watcher,
		lns:        listeners,
		pidfile:    opts.PidFile,
		log:        log,
	}, nil
}

func servicesAddresses(cfg *config.Config) map[string]grace.Addressable {
	a := make(map[string]grace.Addressable)
	cfg.GRPC.ForEachService(func(s *config.Service) {
		a[s.Label] = &addr{address: s.Address.String(), network: s.Network}
	})
	cfg.HTTP.ForEachService(func(s *config.Service) {
		a[s.Label] = &addr{address: s.Address.String(), network: s.Network}
	})
	return a
}

func newServerless(config *config.Config, log *zerolog.Logger) (*rserverless.Serverless, error) {
	sl := make(map[string]rserverless.Service)
	logger := log.With().Str("pkg", "serverless").Logger()
	if err := config.Serverless.ForEach(func(name string, config map[string]any) error {
		new, ok := rserverless.Services[name]
		if !ok {
			return fmt.Errorf("serverless service %s does not exist", name)
		}
		log := logger.With().Str("service", name).Logger()
		svc, err := new(config, &log)
		if err != nil {
			return errors.Wrapf(err, "serverless service %s could not be initialized", name)
		}
		sl[name] = svc
		return nil
	}); err != nil {
		return nil, err
	}

	ss, err := rserverless.New(
		rserverless.WithLogger(&logger),
		rserverless.WithServices(sl),
	)
	if err != nil {
		return nil, err
	}
	return ss, nil
}

func setRandomAddresses(c *config.Config, lns map[string]net.Listener, log *zerolog.Logger) {
	f := func(s *config.Service) {
		if s.Address != "" {
			return
		}
		ln, ok := lns[s.Label]
		if !ok {
			log.Fatal().Msg("port not assigned for service " + s.Label)
		}
		s.SetAddress(config.Address(ln.Addr().String()))
		log.Debug().
			Msgf("set random address %s:%s to service %s", ln.Addr().Network(), ln.Addr().String(), s.Label)
	}
	c.GRPC.ForEachService(f)
	c.HTTP.ForEachService(f)
}

type addr struct {
	address string
	network string
}

func (a *addr) Address() string {
	return a.address
}

func (a *addr) Network() string {
	return a.network
}

func groupGRPCByAddress(cfg *config.Config) []*config.GRPC {
	// TODO: same address cannot be used in different configurations
	g := map[string]*config.GRPC{}
	cfg.GRPC.ForEachService(func(s *config.Service) {
		if _, ok := g[s.Address.String()]; !ok {
			g[s.Address.String()] = &config.GRPC{
				Address:          s.Address,
				Network:          s.Network,
				ShutdownDeadline: cfg.GRPC.ShutdownDeadline,
				EnableReflection: cfg.GRPC.EnableReflection,
				Services:         make(map[string]config.ServicesConfig),
				Interceptors:     cfg.GRPC.Interceptors,
			}
		}
		g[s.Address.String()].Services[s.Name] = config.ServicesConfig{
			{Config: s.Config, Address: s.Address, Network: s.Network, Label: s.Label},
		}
	})
	l := make([]*config.GRPC, 0, len(g))
	for _, c := range g {
		l = append(l, c)
	}
	return l
}

func groupHTTPByAddress(cfg *config.Config) []*config.HTTP {
	g := map[string]*config.HTTP{}
	cfg.HTTP.ForEachService(func(s *config.Service) {
		if _, ok := g[s.Address.String()]; !ok {
			g[s.Address.String()] = &config.HTTP{
				Address:     s.Address,
				Network:     s.Network,
				CertFile:    cfg.HTTP.CertFile,
				KeyFile:     cfg.HTTP.KeyFile,
				Services:    make(map[string]config.ServicesConfig),
				Middlewares: cfg.HTTP.Middlewares,
			}
		}
		g[s.Address.String()].Services[s.Name] = config.ServicesConfig{
			{Config: s.Config, Address: s.Address, Network: s.Network, Label: s.Label},
		}
	})
	l := make([]*config.HTTP, 0, len(g))
	for _, c := range g {
		l = append(l, c)
	}
	return l
}

// Start starts all the reva services and waits for a signal.
func (r *Reva) Start() error {
	defer r.watcher.Clean()
	r.watcher.SetServers(list.Map(r.servers, func(s *Server) grace.Server { return s.server }))
	r.watcher.SetServerless(r.serverless)

	var g errgroup.Group
	for _, server := range r.servers {
		server := server
		g.Go(func() error {
			return server.Start()
		})
	}

	g.Go(func() error {
		return r.serverless.Start()
	})

	r.watcher.TrapSignals()
	return g.Wait()
}

func initSharedConf(config *config.Config) {
	sharedconf.Init(config.Shared)
}

func initWatcher(filename string, log *zerolog.Logger) (*grace.Watcher, error) {
	return handlePIDFlag(log, filename)
	// TODO(labkode): maybe pidfile can be created later on? like once a server is going to be created?
}

func applyTemplates(config *config.Config) error {
	return config.ApplyTemplates(config)
}

func initCPUCount(conf *config.Core, log *zerolog.Logger) error {
	ncpus, err := adjustCPU(conf.MaxCPUs)
	if err != nil {
		return errors.Wrap(err, "error adjusting number of cpus")
	}
	log.Info().Msgf("running on %d cpus", ncpus)
	return nil
}

func handlePIDFlag(l *zerolog.Logger, pidFile string) (*grace.Watcher, error) {
	w := grace.NewWatcher(
		grace.WithPIDFile(pidFile),
		grace.WithLogger(l.With().Str("pkg", "grace").Logger()),
	)
	err := w.WritePID()
	if err != nil {
		return nil, err
	}

	return w, nil
}

func initTracing(conf *config.Core) {
	if conf.TracingEnabled {
		rtrace.SetTraceProvider(conf.TracingCollector, conf.TracingEndpoint, conf.TracingServiceName)
	}
}

// adjustCPU parses string cpu and sets GOMAXPROCS
// according to its value. It accepts either
// a number (e.g. 3) or a percent (e.g. 50%).
// Default is to use all available cores.
func adjustCPU(cpu string) (int, error) {
	var numCPU int

	availCPU := runtime.NumCPU()

	if cpu != "" {
		if strings.HasSuffix(cpu, "%") {
			// Percent
			var percent float32
			pctStr := cpu[:len(cpu)-1]
			pctInt, err := strconv.Atoi(pctStr)
			if err != nil || pctInt < 1 || pctInt > 100 {
				return 0, fmt.Errorf("invalid CPU value: percentage must be between 1-100")
			}
			percent = float32(pctInt) / 100
			numCPU = int(float32(availCPU) * percent)
		} else {
			// Number
			num, err := strconv.Atoi(cpu)
			if err != nil || num < 1 {
				return 0, fmt.Errorf("invalid CPU value: provide a number or percent greater than 0")
			}
			numCPU = num
		}
	} else {
		numCPU = availCPU
	}

	if numCPU > availCPU || numCPU == 0 {
		numCPU = availCPU
	}

	runtime.GOMAXPROCS(numCPU)
	return numCPU, nil
}

func listenerFromAddress(lns map[string]net.Listener, network string, address config.Address) net.Listener {
	for _, ln := range lns {
		if netutil.AddressEqual(ln.Addr(), network, address.String()) {
			return ln
		}
	}
	panic(fmt.Sprintf("listener not found for address %s:%s", network, address))
}

func newServers(ctx context.Context, grpc []*config.GRPC, http []*config.HTTP, lns map[string]net.Listener, log *zerolog.Logger) ([]*Server, error) {
	servers := make([]*Server, 0, len(grpc)+len(http))
	for _, cfg := range grpc {
		logger := log.With().Str("pkg", "grpc").Logger()
		ctx := appctx.WithLogger(ctx, &logger)
		services, err := rgrpc.InitServices(ctx, cfg.Services)
		if err != nil {
			return nil, err
		}
		unaryChain, streamChain, err := initGRPCInterceptors(cfg.Interceptors, grpcUnprotected(services), log)
		if err != nil {
			return nil, err
		}
		s, err := rgrpc.NewServer(
			rgrpc.EnableReflection(cfg.EnableReflection),
			rgrpc.WithShutdownDeadline(cfg.ShutdownDeadline),
			rgrpc.WithLogger(logger),
			rgrpc.WithServices(services),
			rgrpc.WithUnaryServerInterceptors(unaryChain),
			rgrpc.WithStreamServerInterceptors(streamChain),
		)
		if err != nil {
			return nil, err
		}
		ln := listenerFromAddress(lns, cfg.Network, cfg.Address)
		server := &Server{
			server:   s,
			listener: ln,
			services: maps.MapValues(services, func(s rgrpc.Service) any { return s }),
		}
		log.Debug().
			Interface("services", maps.Keys(cfg.Services)).
			Msgf("spawned grpc server for services listening at %s:%s", ln.Addr().Network(), ln.Addr().String())
		servers = append(servers, server)
	}
	for _, cfg := range http {
		logger := log.With().Str("pkg", "http").Logger()
		ctx := appctx.WithLogger(ctx, &logger)
		services, err := rhttp.InitServices(ctx, cfg.Services)
		if err != nil {
			return nil, err
		}
		middlewares, err := initHTTPMiddlewares(cfg.Middlewares, httpUnprotected(services), &logger)
		if err != nil {
			return nil, err
		}
		s, err := rhttp.New(
			rhttp.WithServices(services),
			rhttp.WithLogger(logger),
			rhttp.WithCertAndKeyFiles(cfg.CertFile, cfg.KeyFile),
			rhttp.WithMiddlewares(middlewares),
		)
		if err != nil {
			return nil, err
		}
		ln := listenerFromAddress(lns, cfg.Network, cfg.Address)
		server := &Server{
			server:   s,
			listener: ln,
			services: maps.MapValues(services, func(s global.Service) any { return s }),
		}
		log.Debug().
			Interface("services", maps.Keys(cfg.Services)).
			Msgf("spawned http server for services listening at %s:%s", ln.Addr().Network(), ln.Addr().String())
		servers = append(servers, server)
	}
	return servers, nil
}
