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

package revadcmd

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"syscall"

	gorun "runtime"

	"github.com/cs3org/reva"
	"github.com/cs3org/reva/cmd/revad/pkg/config"
	"github.com/cs3org/reva/cmd/revad/pkg/grace"
	"github.com/cs3org/reva/cmd/revad/runtime"
	"github.com/cs3org/reva/pkg/logger"
	"github.com/cs3org/reva/pkg/plugin"
	"github.com/cs3org/reva/pkg/sysinfo"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

var (
	versionFlag = flag.Bool("version", false, "show version and exit")
	testFlag    = flag.Bool("t", false, "test configuration and exit")
	signalFlag  = flag.String("s", "", "send signal to a master process: stop, quit, reload")
	configFlag  = flag.String("c", "/etc/revad/revad.toml", "set configuration file")
	pidFlag     = flag.String("p", "", "pid file. If empty defaults to a random file in the OS temporary directory")
	dirFlag     = flag.String("dev-dir", "", "runs any toml file in the specified directory. Intended for development use only")
	pluginsFlag = flag.Bool("plugins", false, "list all the plugins and exit")

	// Compile time variables initialized with gcc flags.
	gitCommit, buildDate, version, goVersion string
)

var (
	revaProcs []*runtime.Reva
)

func Main() {
	flag.Parse()

	initPlugins()

	// initialize the global system information
	if err := sysinfo.InitSystemInfo(&sysinfo.RevaVersion{Version: version, BuildDate: buildDate, GitCommit: gitCommit, GoVersion: goVersion}); err != nil {
		fmt.Fprintf(os.Stderr, "error initializing system info: %s\n", err.Error())
		// This is not really a fatal error, so don't panic
	}

	handleVersionFlag()
	handleSignalFlag()
	handlePluginsFlag()

	confs, err := getConfigs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading the configuration file(s): %s\n", err.Error())
		os.Exit(1)
	}

	// if there is more than one configuration available and
	// the pid flag has been set we abort as the pid flag
	// is meant to work only with one main configuration.
	if len(confs) != 1 && *pidFlag != "" {
		fmt.Fprintf(os.Stderr, "cannot run with multiple configurations and one pid file, remote the -p flag\n")
		os.Exit(1)
	}

	// if test flag is true we exit as this flag only tests for valid configurations.
	if *testFlag {
		os.Exit(0)
	}

	runConfigs(confs)
}

func initPlugins() {
	plugins := reva.GetPlugins("")
	for _, p := range plugins {
		plugin.RegisterPlugin(p.ID.Namespace(), p.ID.Name(), p.New)
	}
}

func handleVersionFlag() {
	if *versionFlag {
		fmt.Fprintf(os.Stderr, "%s\n", getVersionString())
		os.Exit(0)
	}
}

func handlePluginsFlag() {
	if !*pluginsFlag {
		return
	}

	// TODO (gdelmont): maybe in future if needed we can filter
	// by namespace (for example for getting all the http plugins).
	// For now we just list all the plugins.
	plugins := reva.GetPlugins("")
	grouped := groupByNamespace(plugins)

	count := 0
	for ns, plugins := range grouped {
		fmt.Printf("[%s]\n", ns)
		for _, p := range plugins {
			fmt.Printf("%s -> %s\n", p.ID.Name(), pkgOfFunction(p.New))
		}
		count++
		if len(grouped) != count {
			fmt.Println()
		}
	}
	os.Exit(0)
}

func nameOfFunction(f any) string {
	return gorun.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

func pkgOfFunction(f any) string {
	name := nameOfFunction(f)
	i := strings.LastIndex(name, ".")
	return name[:i]
}

func groupByNamespace(plugins []reva.PluginInfo) map[string][]reva.PluginInfo {
	m := make(map[string][]reva.PluginInfo)
	for _, p := range plugins {
		m[p.ID.Namespace()] = append(m[p.ID.Namespace()], p)
	}
	return m
}

func getVersionString() string {
	msg := "version=%s "
	msg += "commit=%s "
	msg += "go_version=%s "
	msg += "build_date=%s"

	return fmt.Sprintf(msg, version, gitCommit, goVersion, buildDate)
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
		case "dump":
			signal = syscall.SIGUSR1
		default:
			fmt.Fprintf(os.Stderr, "unknown signal %q\n", *signalFlag)
			os.Exit(1)
		}

		// check that we have a valid pidfile
		if *pidFlag == "" {
			fmt.Fprintf(os.Stderr, "-s flag not set, no clue where the pidfile is stored. Check the logs for its location.\n")
			os.Exit(1)
		}
		process, err := grace.GetProcessFromFile(*pidFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting process from pidfile: %v\n", err)
			os.Exit(1)
		}

		// kill process with signal
		if err := process.Signal(signal); err != nil {
			fmt.Fprintf(os.Stderr, "error signaling process %d with signal %s: %v\n", process.Pid, signal, err)
			os.Exit(1)
		}

		os.Exit(0)
	}
}

func getConfigs() ([]*config.Config, error) {
	var confs []string
	// give priority to read from dev-dir
	if *dirFlag != "" {
		cfgs, err := getConfigsFromDir(*dirFlag)
		if err != nil {
			return nil, err
		}
		confs = append(confs, cfgs...)
	} else {
		confs = append(confs, *configFlag)
	}

	// if we don't have a config file we abort
	if len(confs) == 0 {
		fmt.Fprintf(os.Stderr, "no configuration found\n")
		os.Exit(1)
	}

	configs, err := readConfigs(confs)
	if err != nil {
		return nil, err
	}

	return configs, nil
}

func getConfigsFromDir(dir string) (confs []string, err error) {
	entries, err := os.ReadDir(*dirFlag)
	if err != nil {
		return nil, err
	}
	files := make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, info)
	}

	for _, value := range files {
		if !value.IsDir() {
			expr := regexp.MustCompile(`[\w].toml`)
			if expr.Match([]byte(value.Name())) {
				confs = append(confs, path.Join(dir, value.Name()))
			}
		}
	}
	return
}

func readConfigs(files []string) ([]*config.Config, error) {
	confs := make([]*config.Config, 0, len(files))
	for _, conf := range files {
		fd, err := os.Open(conf)
		if err != nil {
			return nil, err
		}
		defer fd.Close()

		c, err := config.Load(fd)
		if err != nil {
			return nil, err
		}
		confs = append(confs, c)
	}
	return confs, nil
}

func runConfigs(confs []*config.Config) {
	pidfile := getPidfile()
	if len(confs) == 1 {
		runSingle(confs[0], pidfile)
		return
	}

	runMultiple(confs)
}

func registerReva(r *runtime.Reva) {
	revaProcs = append(revaProcs, r)
}

func runSingle(conf *config.Config, pidfile string) {
	log := initLogger(conf.Log)
	reva, err := runtime.New(conf,
		runtime.WithPidFile(pidfile),
		runtime.WithLogger(log),
	)
	if err != nil {
		abort(log, "error creating reva runtime: %v", err)
	}
	registerReva(reva)
	if err := reva.Start(); err != nil {
		abort(log, "error starting reva: %v", err)
	}
}

func abort(log *zerolog.Logger, format string, a ...any) {
	log.Fatal().Msgf(format, a...)
}

func runMultiple(confs []*config.Config) {
	var wg sync.WaitGroup

	for _, conf := range confs {
		wg.Add(1)
		pidfile := getPidfile()
		go func(wg *sync.WaitGroup, conf *config.Config) {
			defer wg.Done()
			runSingle(conf, pidfile)
		}(&wg, conf)
	}
	wg.Wait()
	os.Exit(0)
}

func getPidfile() string {
	uuid := uuid.New().String()
	name := fmt.Sprintf("revad-%s.pid", uuid)

	return path.Join(os.TempDir(), name)
}

func initLogger(conf *config.Log) *zerolog.Logger {
	log, err := newLogger(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating logger: %v", err)
		os.Exit(1)
	}
	return log
}

func newLogger(conf *config.Log) (*zerolog.Logger, error) {
	// TODO(labkode): use debug level rather than info as default until reaching a stable version.
	// Helps having smaller development files.
	if conf.Level == "" {
		conf.Level = zerolog.DebugLevel.String()
	}

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

	fd, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "error creating log file: "+out)
	}

	return fd, nil
}
