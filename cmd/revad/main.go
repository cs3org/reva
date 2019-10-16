// Copyright 2018-2019 CERN
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

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"contrib.go.opencensus.io/exporter/jaeger"
	"github.com/cs3org/reva/cmd/revad/internal/config"
	"github.com/cs3org/reva/cmd/revad/internal/grace"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/rgrpc"
	"github.com/cs3org/reva/pkg/rhttp"
	"github.com/gofrs/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

var (
	versionFlag = flag.Bool("version", false, "show version and exit")
	testFlag    = flag.Bool("t", false, "test configuration and exit")
	signalFlag  = flag.String("s", "", "send signal to a master process: stop, quit, reload")
	configFlag  = flag.String("c", "/etc/revad/revad.toml", "set configuration file")
	pidFlag     = flag.String("p", "", "pid file. If empty defaults to a random file in the OS temporary directory")

	// Compile time variables initialez with gcc flags.
	gitCommit, gitBranch, buildDate, version, goVersion, buildPlatform string
)

type coreConf struct {
	MaxCPUs            string `mapstructure:"max_cpus"`
	TracingEnabled     bool   `mapstructure:"tracing_enabled"`
	TracingEndpoint    string `mapstructure:"tracing_endpoint"`
	TracingCollector   string `mapstructure:"tracing_collector"`
	TracingServiceName string `mapstructure:"tracing_service_name"`
}

func main() {
	flag.Parse()

	handleVersionFlag()
	handleSignalFlag()
	handleTestFlag()

	mainConf := handleConfigFlagOrDie()
	coreConf := parseCoreConfOrDie(mainConf["core"])
	logConf := parseLogConfOrDie(mainConf["log"])

	log, err := newLogger(logConf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating logger, exiting ...")
		os.Exit(1)
	}

	if err := setupOpenCensus(coreConf); err != nil {
		log.Error().Err(err).Msg("error configuring open census stats and tracing")
		os.Exit(1)
	}

	ncpus, err := adjustCPU(coreConf.MaxCPUs)
	if err != nil {
		log.Error().Err(err).Msg("error adjusting number of cpus")
		os.Exit(1)
	}
	log.Info().Msgf("%s", getVersionString())
	log.Info().Msgf("running on %d cpus", ncpus)

	servers := map[string]grace.Server{}
	if isEnabledHTTP(mainConf) {
		s, err := getHTTPServer(mainConf["http"], log)
		if err != nil {
			log.Error().Err(err).Msg("error creating http server")
			os.Exit(1)
		}
		servers["http"] = s
	}

	if isEnabledGRPC(mainConf) {
		s, err := getGRPCServer(mainConf["grpc"], log)
		if err != nil {
			log.Error().Err(err).Msg("error creating grpc server")
			os.Exit(1)
		}
		servers["grpc"] = s
	}

	if len(servers) == 0 {
		// nothing to do
		log.Info().Msg("nothing to do, no grpc/http enabled_services declared in config")
		os.Exit(1)
	}

	watcher, err := handlePIDFlag(log) // TODO(labkode): maybe pidfile can be created later on? like once a server is going to be created?
	if err != nil {
		log.Error().Err(err).Msg("error creating grace watcher")
		os.Exit(1)
	}

	listeners, err := watcher.GetListeners(servers)
	if err != nil {
		log.Error().Err(err).Msg("error getting sockets")
		watcher.Exit(1)
	}

	if isEnabledHTTP(mainConf) {
		go func() {
			if err := servers["http"].(*rhttp.Server).Start(listeners["http"]); err != nil {
				log.Error().Err(err).Msg("error starting the http server")
				watcher.Exit(1)
			}
		}()
	}

	if isEnabledGRPC(mainConf) {
		go func() {
			if err := servers["grpc"].(*rgrpc.Server).Start(listeners["grpc"]); err != nil {
				log.Error().Err(err).Msg("error starting the grpc server")
				watcher.Exit(1)
			}
		}()
	}

	// wait for signal to close servers
	watcher.TrapSignals()
}

func newLogger(conf *logConf) (*zerolog.Logger, error) {
	var opts []logger.Option
	opts = append(opts, logger.WithLevel(conf.Level))

	w, err := getWriter(conf.Output)
	if err != nil {
		return nil, err
	}

	opts = append(opts, logger.WithWriter(w, logger.Mode(conf.Mode)))

	l := logger.New(opts...)
	sub := l.With().Int("pid", os.Getpid()).Logger()
	return &sub, nil
}

func getWriter(out string) (io.Writer, error) {
	if out == "stderr" || out == "" {
		return os.Stderr, nil
	}

	if out == "stdout" {
		return os.Stdout, nil
	}

	fd, err := os.Create(out)
	if err != nil {
		err = errors.Wrap(err, "error creating log file: "+out)
		return nil, err
	}

	return fd, nil
}

func getVersionString() string {
	msg := "version=%s "
	msg += "commit=%s "
	msg += "branch=%s "
	msg += "go_version=%s "
	msg += "build_date=%s "
	msg += "build_platform=%s"

	return fmt.Sprintf(msg, version, gitCommit, gitBranch, goVersion, buildDate, buildPlatform)
}

func handleVersionFlag() {
	if *versionFlag {
		fmt.Fprintf(os.Stderr, "%s\n", getVersionString())
		os.Exit(1)
	}
}

func handleSignalFlag() {
	if *signalFlag != "" {
		var signal syscall.Signal
		switch *signalFlag {
		case "reload":
			signal = syscall.SIGHUP
		case "quit":
			signal = syscall.SIGQUIT
		case "stop":
			signal = syscall.SIGTERM
		default:
			fmt.Fprintf(os.Stderr, "unknown signal %q\n", *signalFlag)
			os.Exit(1)
		}

		// check that we have a valid pidfile
		if *pidFlag == "" {
			fmt.Fprintf(os.Stderr, "-s flag not set, no clue where the pidfile is stored\n")
			os.Exit(1)
		}
		process, err := grace.GetProcessFromFile(*pidFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting process from pidfile: %v\n", err)
			os.Exit(1)
		}

		// kill process with signal
		if err := process.Signal(signal); err != nil {
			fmt.Fprintf(os.Stderr, "error signaling process %d with signal %s\n", process.Pid, signal)
			os.Exit(1)
		}

		os.Exit(0)
	}
}

func handleTestFlag() {
	if *testFlag {
		os.Exit(0)
	}
}

func handlePIDFlag(l *zerolog.Logger) (*grace.Watcher, error) {
	// if pid is empty, we store it in the OS temporary folder with random name
	if *pidFlag == "" {
		uuid := uuid.Must(uuid.NewV4())
		*pidFlag = path.Join(os.TempDir(), "revad-"+uuid.String()+".pid")
	}

	var opts []grace.Option
	opts = append(opts, grace.WithPIDFile(*pidFlag))
	opts = append(opts, grace.WithLogger(l.With().Str("pkg", "grace").Logger()))
	w := grace.NewWatcher(opts...)
	err := w.WritePID()
	if err != nil {
		return nil, err
	}

	return w, nil
}

func getGRPCServer(conf interface{}, l *zerolog.Logger) (*rgrpc.Server, error) {
	sub := l.With().Str("pkg", "rgrpc").Logger()
	s, err := rgrpc.NewServer(conf, sub)
	if err != nil {
		err = errors.Wrap(err, "main: error creating grpc server")
		return nil, err
	}
	return s, nil
}

func getHTTPServer(conf interface{}, l *zerolog.Logger) (*rhttp.Server, error) {
	sub := l.With().Str("pkg", "rhttp").Logger()
	s, err := rhttp.New(conf, sub)
	if err != nil {
		err = errors.Wrap(err, "main: error creating http server")
		return nil, err
	}
	return s, nil
}

func handleConfigFlagOrDie() map[string]interface{} {
	fd, err := os.Open(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %+s\n", err.Error())
		os.Exit(1)
	}
	defer fd.Close()

	v, err := config.Read(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading config: %s\n", err.Error())
		os.Exit(1)
	}

	return v
}

func setupOpenCensus(conf *coreConf) error {
	if err := view.Register(ochttp.DefaultServerViews...); err != nil {
		return err
	}

	if err := view.Register(ocgrpc.DefaultServerViews...); err != nil {
		return err
	}

	if !conf.TracingEnabled {
		return nil
	}

	if conf.TracingEndpoint == "" {
		conf.TracingEndpoint = "localhost:6831"
	}

	if conf.TracingCollector == "" {
		conf.TracingCollector = "http://localhost:14268/api/traces"
	}

	if conf.TracingServiceName == "" {
		conf.TracingServiceName = "revad"
	}

	je, err := jaeger.NewExporter(jaeger.Options{
		AgentEndpoint:     conf.TracingEndpoint,
		CollectorEndpoint: conf.TracingCollector,
		ServiceName:       conf.TracingServiceName,
	})

	if err != nil {
		return err
	}

	// register it as a trace exporter
	trace.RegisterExporter(je)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	return nil
}

//  adjustCPU parses string cpu and sets GOMAXPROCS
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

func parseCoreConfOrDie(v interface{}) *coreConf {
	c := &coreConf{}
	if err := mapstructure.Decode(v, c); err != nil {
		fmt.Fprintf(os.Stderr, "error decoding core config: %s\n", err.Error())
		os.Exit(1)
	}
	return c
}

func parseLogConfOrDie(v interface{}) *logConf {
	c := &logConf{}
	if err := mapstructure.Decode(v, c); err != nil {
		fmt.Fprintf(os.Stderr, "error decoding log config: %s\n", err.Error())
		os.Exit(1)
	}

	// if mode is not set, we use console mode, easier for devs
	if c.Mode == "" {
		c.Mode = "console"
	}

	return c
}

type logConf struct {
	Output string `mapstructure:"output"`
	Mode   string `mapstructure:"mode"`
	Level  string `mapstructure:"level"`
}

func isEnabledHTTP(conf map[string]interface{}) bool {
	return isEnabled("http", conf)
}

func isEnabledGRPC(conf map[string]interface{}) bool {
	return isEnabled("grpc", conf)
}

func isEnabled(key string, conf map[string]interface{}) bool {
	if a, ok := conf[key]; ok {
		if b, ok := a.(map[string]interface{}); ok {
			if c, ok := b["enabled_services"]; ok {
				if d, ok := c.([]interface{}); ok {
					if len(d) > 0 {
						return true
					}
				}
			}
		}
	}
	return false
}
