// Copyright 2018-2024 CERN
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
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/v3/internal/grpc/control"
	"github.com/cs3org/reva/v3/pkg/appctx"
	"github.com/cs3org/reva/v3/pkg/invoke"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/rgrpc"
	"github.com/cs3org/reva/v3/pkg/rhttp"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/cs3org/reva/v3/pkg/trace"

	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/rserverless"
	"github.com/cs3org/reva/v3/pkg/sharedconf"
	"github.com/cs3org/reva/v3/pkg/utils/list"
	"github.com/cs3org/reva/v3/pkg/utils/maps"
	netutil "github.com/cs3org/reva/v3/pkg/utils/net"
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

	// registry is this instance's service registry.
	registry          registry.Registry
	heartbeatInterval time.Duration
	// controlAddr is the advertised address of this process's control channel
	// (empty if the process hosts nothing invokable).
	controlAddr string

	pidfile       string
	log           *zerolog.Logger
	traceShutdown func(context.Context) error
}

// Server represents a reva server (grpc or http).
type Server struct {
	server   grace.Server
	listener net.Listener

	// transport is "grpc" or "http"; advertised in registry node metadata.
	transport string
	// scheme is "http" or "https" for HTTP servers (empty for grpc).
	scheme string
	// internal marks a server not advertised as a registry service (the
	// per-process control channel).
	internal bool

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

	traceShutdown, err := trace.InitProvider(ctx, trace.ProviderConfig{
		Enabled:     config.Core.TracingEnabled,
		Endpoint:    config.Core.TracingEndpoint,
		Collector:   config.Core.TracingCollector,
		ServiceName: config.Core.TracingServiceName,
		Log:         log,
	})
	if err != nil {
		watcher.Clean()
		return nil, errors.Wrap(err, "error initializing tracing provider")
	}

	reg, err := buildRegistry(config, opts.Registry, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	// Install the process-wide resolver before constructing services, so any
	// service that resolves a peer at construction time finds it (first wins).
	service.SetGlobal(service.NewClients(reg))
	// Expose the registry to the Admin API for fleet introspection.
	service.SetGlobalRegistry(reg)

	grpc := groupGRPCByAddress(config)
	http := groupHTTPByAddress(config)
	servers, err := newServers(ctx, grpc, http, listeners, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	serverless, err := newServerless(ctx, config, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}

	// Stand up this process's control channel on its own port, once every
	// service (and thus every Invokable) has been constructed.
	controlSrv, controlAddr, err := newControlServer(config, log)
	if err != nil {
		watcher.Clean()
		return nil, err
	}
	if controlSrv != nil {
		servers = append(servers, controlSrv)
	}

	r := &Reva{
		ctx:               ctx,
		config:            config,
		servers:           servers,
		serverless:        serverless,
		watcher:           watcher,
		lns:               listeners,
		registry:          reg,
		heartbeatInterval: parseDurationOr(config.Shared.Registry.HeartbeatInterval, 5*time.Second),
		controlAddr:       controlAddr,
		pidfile:           opts.PidFile,
		log:               log,
		traceShutdown:     traceShutdown,
	}

	// Self-register every loaded service after listeners have bound.
	r.register()

	r.initConfigDumper()
	return r, nil
}

// parseDurationOr parses a Go duration string, returning def on empty/invalid.
func parseDurationOr(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func (r *Reva) initConfigDumper() {
	// dump the config when the process receives a SIGUSR1 signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGUSR1)

	go func() {
		for {
			<-sigs
			r.dumpConfig()
		}
	}()
}

func (r *Reva) dumpConfig() {
	cfg := r.config.Dump()
	out := r.config.Core.ConfigDumpFile
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		r.log.Error().Err(err).Msgf("error opening file %s for dumping the config", out)
		return
	}
	defer f.Close()

	enc := toml.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		r.log.Error().Err(err).Msg("error encoding config")
		return
	}
	r.log.Debug().Msgf("config dumped successfully in %s", out)
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

func newServerless(ctx context.Context, config *config.Config, log *zerolog.Logger) (*rserverless.Serverless, error) {
	sl := make(map[string]rserverless.Service)
	logger := log.With().Str("pkg", "serverless").Logger()
	if err := config.Serverless.ForEach(func(name string, config map[string]any) error {
		new, ok := rserverless.Services[name]
		if !ok {
			return fmt.Errorf("serverless service %s does not exist", name)
		}
		log := logger.With().Str("service", name).Logger()
		ctx := appctx.WithLogger(ctx, &log)
		svc, err := new(ctx, config)
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
	defer r.traceShutdown(r.ctx)
	defer r.watcher.Clean()
	// Drain this instance's registry entries on graceful shutdown.
	defer r.deregister()
	// Keep registered nodes live while we serve.
	r.startHeartbeat()
	r.watcher.SetServers(list.Map(r.servers, func(s *Server) grace.Server { return s.server }))
	r.watcher.SetServerless(r.serverless)

	var g errgroup.Group
	for _, server := range r.servers {
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

// captureInstances registers each loaded service as an invocation instance
// under its node id, so the control channel can route to the exact instance
// even when a process hosts several of the same service.
func captureInstances[T any](services map[string]T, cfgServices map[string]config.ServicesConfig, addr string) {
	for name, impl := range services {
		var conf map[string]any
		if sc := cfgServices[name]; len(sc) > 0 {
			conf = sc[0].Config
		}
		var inv invoke.Invokable
		if i, ok := any(impl).(invoke.Invokable); ok {
			inv = i
		}
		invoke.RegisterInstance(nodeID(addr, name), name, conf, inv)
	}
}

func newServers(ctx context.Context, grpc []*config.GRPC, http []*config.HTTP, lns map[string]net.Listener, log *zerolog.Logger) ([]*Server, error) {
	hostname, _ := os.Hostname()
	servers := make([]*Server, 0, len(grpc)+len(http))
	for _, cfg := range grpc {
		logger := log.With().Str("pkg", "grpc").Logger()
		ctx := appctx.WithLogger(ctx, &logger)
		services, err := rgrpc.InitServices(ctx, cfg.Services)
		if err != nil {
			return nil, err
		}
		ln := listenerFromAddress(lns, cfg.Network, cfg.Address)
		captureInstances(services, cfg.Services, hostPort(hostname, ln.Addr().String()))
		unaryChain, streamChain, err := initGRPCInterceptors(cfg.Interceptors, grpcUnprotected(cfg.EnableReflection, services), log)
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
		server := &Server{
			server:    s,
			listener:  ln,
			transport: "grpc",
			services:  maps.MapValues(services, func(s rgrpc.Service) any { return s }),
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
		ln := listenerFromAddress(lns, cfg.Network, cfg.Address)
		captureInstances(services, cfg.Services, hostPort(hostname, ln.Addr().String()))
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
		scheme := "http"
		if cfg.CertFile != "" && cfg.KeyFile != "" {
			scheme = "https"
		}
		server := &Server{
			server:    s,
			listener:  ln,
			transport: "http",
			scheme:    scheme,
			services:  maps.MapValues(services, func(s global.Service) any { return s }),
		}
		log.Debug().
			Interface("services", maps.Keys(cfg.Services)).
			Msgf("spawned http server for services listening at %s:%s", ln.Addr().Network(), ln.Addr().String())
		servers = append(servers, server)
	}
	return servers, nil
}

// newControlServer builds this process's control channel: a dedicated gRPC
// server hosting only reva.control.v1beta1, so every invocation a service exposes
// — the shared defaults (e.g. config) and any it defines via Invokable — is
// reachable on a single extra port. It is created whenever the process hosts
// anything invokable (any configured service, since each gets the shared
// defaults) and is independent of whether the Admin API service is loaded. Its
// methods require the admin scope, enforced by the standard auth interceptor
// chain, so the channel is as protected as the Admin API itself. It returns the
// server and the address to advertise, or (nil, "", nil) when none is needed.
func newControlServer(cfg *config.Config, log *zerolog.Logger) (*Server, string, error) {
	if !invoke.HasInvocations() {
		return nil, "", nil
	}
	bind := cfg.GRPC.ControlAddress
	if bind == "" {
		// A random port on every interface; the auth interceptor, not the bind
		// host, is what protects it.
		bind = "0.0.0.0:0"
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return nil, "", fmt.Errorf("runtime: binding control channel on %q: %w", bind, err)
	}
	logger := log.With().Str("pkg", "grpc").Str("server", "control").Logger()
	unaryChain, streamChain, err := initGRPCInterceptors(nil, nil, &logger)
	if err != nil {
		_ = ln.Close()
		return nil, "", err
	}
	services := map[string]rgrpc.Service{service.NameControl: control.New()}
	s, err := rgrpc.NewServer(
		rgrpc.WithLogger(logger),
		rgrpc.WithServices(services),
		rgrpc.WithUnaryServerInterceptors(unaryChain),
		rgrpc.WithStreamServerInterceptors(streamChain),
	)
	if err != nil {
		_ = ln.Close()
		return nil, "", err
	}
	hostname, _ := os.Hostname()
	addr := hostPort(hostname, ln.Addr().String())
	server := &Server{
		server:    s,
		listener:  ln,
		transport: "grpc",
		internal:  true,
		services:  maps.MapValues(services, func(s rgrpc.Service) any { return s }),
	}
	log.Info().Str("address", addr).Msg("spawned per-process control channel")
	return server, addr, nil
}
