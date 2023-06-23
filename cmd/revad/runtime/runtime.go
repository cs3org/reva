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
	"fmt"
	"net"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"

	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/pkg/sharedconf"
	"github.com/cs3org/reva/pkg/utils/maps"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
)

type Reva struct {
	config *config.Config

	servers []*Server
	watcher *grace.Watcher
	lns     map[string]net.Listener
}

func New(config *config.Config) (*Reva, error) {
	_ = initLogger(config.Log)
	initSharedConf(config)

	pidfile := getPidfile()

	grpc, addrGRPC := groupGRPCByAddress(config)
	http, addrHTTP := groupHTTPByAddress(config)

	log := zerolog.Nop()
	watcher, err := initWatcher(&log, pidfile)
	if err != nil {
		return nil, err
	}

	listeners := initListeners(watcher, maps.Merge(addrGRPC, addrHTTP), &log)
	setRandomAddresses(config, listeners)
	applyTemplates(config)

	servers, err := newServers(grpc, http)
	if err != nil {
		return nil, err
	}

	return &Reva{
		config:  config,
		servers: servers,
		watcher: watcher,
		lns:     listeners,
	}, nil
}

func setRandomAddresses(c *config.Config, lns map[string]net.Listener) {
	f := func(s *config.Service) {
		if s.Address != "" {
			return
		}
		ln, ok := lns[s.Label]
		if !ok {
			abort("port not assigned for service %s", s.Label)
		}
		s.SetAddress(ln.Addr().String())
	}
	c.GRPC.ForEachService(f)
	c.HTTP.ForEachService(f)
}

func initListeners(watcher *grace.Watcher, servers map[string]grace.Addressable, log *zerolog.Logger) map[string]net.Listener {
	listeners, err := watcher.GetListeners(servers)
	if err != nil {
		log.Error().Err(err).Msg("error getting sockets")
		watcher.Exit(1)
	}
	return listeners
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

func groupGRPCByAddress(cfg *config.Config) (map[string]*config.GRPC, map[string]grace.Addressable) {
	// TODO: same address cannot be used in different configurations
	g := map[string]*config.GRPC{}
	a := map[string]grace.Addressable{}
	cfg.GRPC.ForEachService(func(s *config.Service) {
		if _, ok := g[s.Address]; !ok {
			g[s.Address] = &config.GRPC{
				Address:          s.Address,
				Network:          "tcp", // TODO: configure this as well
				ShutdownDeadline: cfg.GRPC.ShutdownDeadline,
				EnableReflection: cfg.GRPC.EnableReflection,
				Services:         make(map[string]config.ServicesConfig),
				Interceptors:     cfg.GRPC.Interceptors,
			}
			if s.Address == "" {
				a[s.Label] = &addr{address: s.Address, network: "tcp"}
			}
		}
		g[s.Address].Services[s.Name] = config.ServicesConfig{
			{Config: s.Config},
		}
	})
	return g, a
}

func groupHTTPByAddress(cfg *config.Config) (map[string]*config.HTTP, map[string]grace.Addressable) {
	g := map[string]*config.HTTP{}
	a := map[string]grace.Addressable{}
	cfg.HTTP.ForEachService(func(s *config.Service) {
		if _, ok := g[s.Address]; !ok {
			g[s.Address] = &config.HTTP{
				Address:     s.Address,
				Network:     "tpc", // TODO: configure this as well
				CertFile:    cfg.HTTP.CertFile,
				KeyFile:     cfg.HTTP.KeyFile,
				Services:    make(map[string]config.ServicesConfig),
				Middlewares: cfg.HTTP.Middlewares,
			}
			if s.Address == "" {
				a[s.Label] = &addr{address: s.Address, network: "tcp"}
			}
		}
		g[s.Address].Services[s.Name] = config.ServicesConfig{
			{Config: s.Config},
		}
	})
	return g, a
}

func (r *Reva) Start() {
	var g errgroup.Group
	for _, server := range r.servers {
		server := server
		g.Go(func() error {
			return server.Start()
		})
	}

	r.watcher.TrapSignals()
	if err := g.Wait(); err != nil {
		// TODO: log error
	}
}

func getPidfile() string {
	uuid := uuid.New().String()
	name := fmt.Sprintf("revad-%s.pid", uuid)

	return path.Join(os.TempDir(), name)
}

func initSharedConf(config *config.Config) {
	sharedconf.Init(config.Shared)
}

func initWatcher(log *zerolog.Logger, filename string) (*grace.Watcher, error) {
	watcher, err := handlePIDFlag(log, filename)
	// TODO(labkode): maybe pidfile can be created later on? like once a server is going to be created?
	if err != nil {
		log.Error().Err(err).Msg("error creating grace watcher")
		os.Exit(1)
	}
	return watcher, err
}

func assignRandomAddress(configs []*config.Config) (addr []net.Listener) {
	assign := func(s *config.Service) {
		if s.Address == "" {
			random, err := randomListener("tpc") // TODO: take from config
			if err != nil {
				abort("error assigning random port to service %s: %v", s.Name, err)
			}
			s.SetAddress(random.Addr().String())
			addr = append(addr, random)
		}
	}
	for _, c := range configs {
		c.GRPC.ForEachService(assign)
		c.HTTP.ForEachService(assign)
	}
	return
}

func randomListener(network string) (net.Listener, error) {
	return net.Listen(network, ":0")
}

func applyTemplates(config *config.Config) {
	// TODO: we might want to prefer before the actual config in the lookup
	// and then the others
	if err := config.ApplyTemplates(config); err != nil {
		abort("error applying templated to config: %v", err)
	}
}

func initCPUCount(conf *config.Core, log *zerolog.Logger) {
	ncpus, err := adjustCPU(conf.MaxCPUs)
	if err != nil {
		log.Error().Err(err).Msg("error adjusting number of cpus")
		os.Exit(1)
	}
	// log.Info().Msgf("%s", getVersionString())
	log.Info().Msgf("running on %d cpus", ncpus)
}

func abort(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(1)
}

func handlePIDFlag(l *zerolog.Logger, pidFile string) (*grace.Watcher, error) {
	var opts []grace.Option
	opts = append(opts, grace.WithPIDFile(pidFile))
	opts = append(opts, grace.WithLogger(l.With().Str("pkg", "grace").Logger()))
	w := grace.NewWatcher(opts...)
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
