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
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/cs3org/reva/cmd/revad/grace"
	"github.com/cs3org/reva/cmd/revad/grpcserver"
	"github.com/cs3org/reva/cmd/revad/httpserver"

	"github.com/cs3org/reva/cmd/revad/config"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

var (
	versionFlag = flag.Bool("version", false, "show version and exit")
	testFlag    = flag.Bool("t", false, "test configuration and exit")
	signalFlag  = flag.String("s", "", "send signal to a master process: stop, quit, reload")
	configFlag  = flag.String("c", "/etc/revad/revad.toml", "set configuration file")
	pidFlag     = flag.String("p", "/var/run/revad.pid", "pid file")

	// Compile time variables initialez with gcc flags.
	gitCommit, gitBranch, buildDate, version, goVersion, buildPlatform string
)

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

	watcher, err := handlePIDFlag(log) // TODO(labkode): maybe pidfile can be created later on?
	if err != nil {
		log.Error().Err(err).Msg("error creating grace watcher")
		os.Exit(1)
	}

	ncpus, err := adjustCPU(coreConf.MaxCPUs)
	if err != nil {
		log.Error().Err(err).Msg("error adjusting number of cpus")
		watcher.Exit(1)
	}
	log.Info().Msgf("%s", getVersionString())
	log.Info().Msgf("running on %d cpus", ncpus)

	servers := []grace.Server{}
	if !coreConf.DisableHTTP {
		s, err := getHTTPServer(mainConf["http"], log)
		if err != nil {
			log.Error().Err(err).Msg("error creating http server")
			watcher.Exit(1)
		}
		servers = append(servers, s)
	}

	if !coreConf.DisableGRPC {
		s, err := getGRPCServer(mainConf["grpc"], log)
		if err != nil {
			log.Error().Err(err).Msg("error creating grpc server")
			watcher.Exit(1)
		}
		servers = append(servers, s)
	}

	listeners, err := watcher.GetListeners(servers)
	if err != nil {
		log.Error().Err(err).Msg("error getting sockets")
		watcher.Exit(1)
	}

	if !coreConf.DisableHTTP {
		go func() {
			if err := servers[0].(*httpserver.Server).Start(listeners[0]); err != nil {
				log.Error().Err(err).Msg("error starting the http server")
				watcher.Exit(1)
			}
		}()
	}

	if !coreConf.DisableGRPC {
		go func() {
			if err := servers[1].(*grpcserver.Server).Start(listeners[1]); err != nil {
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
		err = errors.Wrap(err, "error creating log file")
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
	msg += "build_platform=%s\n"

	return fmt.Sprintf(msg, version, gitCommit, gitBranch, goVersion, buildDate, buildPlatform)
}

func handleVersionFlag() {
	if *versionFlag {
		fmt.Fprintf(os.Stderr, getVersionString())
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

func getGRPCServer(conf interface{}, l *zerolog.Logger) (*grpcserver.Server, error) {
	sub := l.With().Str("pkg", "grpcserver").Logger()
	s, err := grpcserver.New(conf, sub)
	if err != nil {
		err = errors.Wrap(err, "main: error creating grpc server")
		return nil, err
	}
	return s, nil
}

func getHTTPServer(conf interface{}, l *zerolog.Logger) (*httpserver.Server, error) {
	sub := l.With().Str("pkg", "httpserver").Logger()
	s, err := httpserver.New(conf, sub)
	if err != nil {
		err = errors.Wrap(err, "main: error creating http server")
		return nil, err
	}
	return s, nil
}

func handleConfigFlagOrDie() map[string]interface{} {
	fd, err := os.Open(*configFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening file: %+v\n", err)
		os.Exit(1)
	}
	defer fd.Close()

	v, err := config.Read(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading config: %+v\n", err)
		os.Exit(1)
	}

	return v
}

//  adjustCPU parses string cpu and sets GOMAXPROCS
// according to its value. It accepts either
// a number (e.g. 3) or a percent (e.g. 50%).
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
		fmt.Fprintf(os.Stderr, "error decoding core config: %s\n", err)
		os.Exit(1)
	}
	return c
}

type coreConf struct {
	MaxCPUs     string `mapstructure:"max_cpus"`
	LogFile     string `mapstructure:"log_file"`
	LogMode     string `mapstructure:"log_mode"`
	DisableHTTP bool   `mapstructure:"disable_http"`
	DisableGRPC bool   `mapstructure:"disable_grpc"`
}

func parseLogConfOrDie(v interface{}) *logConf {
	c := &logConf{}
	if err := mapstructure.Decode(v, c); err != nil {
		fmt.Fprintf(os.Stderr, "error decoding log config: %s\n", err)
		os.Exit(1)
	}
	return c
}

type logConf struct {
	Output string `mapstructure:"output"`
	Mode   string `mapstructure:"mode"`
	Level  string `mapstructure:"level"`
}
