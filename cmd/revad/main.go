package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"

	"github.com/cernbox/reva/cmd/revad/config"
	"github.com/cernbox/reva/cmd/revad/grace"
	"github.com/cernbox/reva/cmd/revad/grpcsvr"
	"github.com/cernbox/reva/cmd/revad/httpsvr"

	"github.com/mitchellh/mapstructure"
)

var (
	errors = err.New("main")
	logger = log.New("main")
	ctx    = context.Background()
	conf   *coreConfig

	versionFlag = flag.Bool("v", false, "show version and exit")
	testFlag    = flag.Bool("t", false, "test configuration and exit")
	signalFlag  = flag.String("s", "", "send signal to a master process: stop, quit, reopen, reload")
	fileFlag    = flag.String("c", "/etc/revad/revad.toml", "set configuration file")
	pidFlag     = flag.String("p", "/var/run/revad.pid", "pid file")

	// provided at compile time
	GitCommit, GitBranch, GitState, GitSummary, BuildDate, Version string
)

func init() {
	checkFlags()
	writePIDFile()
	readConfig()
	log.Out = getLogOutput(conf.LogFile)
	log.Mode = conf.LogMode
	if err := log.EnableAll(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		grace.Exit(1)
	}
}

func main() {
	tweakCPU()
	printLoggedPkgs()

	grpcSvr := getGRPCServer()
	httpSvr := getHTTPServer()
	servers := []grace.Server{grpcSvr, httpSvr}
	listeners, err := grace.GetListeners(servers)
	if err != nil {
		logger.Error(ctx, err)
		grace.Exit(1)
	}

	go func() {
		if err := grpcSvr.Start(listeners[0]); err != nil {
			err = errors.Wrap(err, "error starting grpc server")
			logger.Error(ctx, err)
			grace.Exit(1)
		}
	}()

	go func() {
		if err := httpSvr.Start(listeners[1]); err != nil {
			err = errors.Wrap(err, "error starting http server")
			logger.Error(ctx, err)
			grace.Exit(1)
		}
	}()

	grace.TrapSignals()
}

func getGRPCServer() *grpcsvr.Server {
	s, err := grpcsvr.New(config.Get("grpc"))
	if err != nil {
		logger.Error(ctx, err)
		grace.Exit(1)
	}
	return s
}

func getHTTPServer() *httpsvr.Server {
	s, err := httpsvr.New(config.Get("http"))
	if err != nil {
		logger.Error(ctx, err)
		grace.Exit(1)
	}
	return s
}

func checkFlags() {
	flag.Parse()

	if *versionFlag {
		msg := "Version: %s\n"
		msg += "GitCommit: %s\n"
		msg += "GitBranch: %s\n"
		msg += "GitSummary: %s\n"
		msg += "BuildDate: %s\n"
		fmt.Printf(msg, Version, GitCommit, GitBranch, GitSummary, BuildDate)
		grace.Exit(1)
	}

	if *fileFlag != "" {
		config.SetFile(*fileFlag)
	}

	if *testFlag {
		err := config.Read()
		if err != nil {
			fmt.Println("unable to read configuration file: ", *fileFlag, err)
			grace.Exit(1)
		}
		grace.Exit(0)
	}

	if *signalFlag != "" {
		fmt.Println("signaling master process")
		grace.Exit(1)
	}
}

func readConfig() {
	err := config.Read()
	if err != nil {
		fmt.Println("unable to read configuration file:", *fileFlag, err)
		grace.Exit(1)
	}

	// get core config

	conf = &coreConfig{}
	if err := mapstructure.Decode(config.Get("core"), conf); err != nil {
		fmt.Fprintln(os.Stderr, "unable to parse core config:", err)
		grace.Exit(1)
	}
}

//  tweakCPU parses string cpu and sets GOMAXPROCS
// according to its value. It accepts either
// a number (e.g. 3) or a percent (e.g. 50%).
func tweakCPU() error {
	cpu := conf.MaxCPUs
	var numCPU int

	availCPU := runtime.NumCPU()

	if strings.HasSuffix(cpu, "%") {
		// Percent
		var percent float32
		pctStr := cpu[:len(cpu)-1]
		pctInt, err := strconv.Atoi(pctStr)
		if err != nil || pctInt < 1 || pctInt > 100 {
			return errors.New("invalid CPU value: percentage must be between 1-100")
		}
		percent = float32(pctInt) / 100
		numCPU = int(float32(availCPU) * percent)
	} else {
		// Number
		num, err := strconv.Atoi(cpu)
		if err != nil || num < 1 {
			return errors.New("invalid CPU value: provide a number or percent greater than 0")
		}
		numCPU = num
	}

	if numCPU > availCPU {
		numCPU = availCPU
	}

	logger.Printf(ctx, "running on %d cpus", numCPU)
	runtime.GOMAXPROCS(numCPU)
	return nil
}

func writePIDFile() {
	err := grace.WritePIDFile(*pidFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		grace.Exit(1)
	}
}

type coreConfig struct {
	MaxCPUs string `mapstructure:"max_cpus"`
	LogFile string `mapstructure:"log_file"`
	LogMode string `mapstructure:"log_mode"`
}

func getLogOutput(val string) io.Writer {
	return os.Stderr
}

func printLoggedPkgs() {
	pkgs := log.ListEnabledPackages()
	for k := range pkgs {
		logger.Printf(ctx, "logging enabled for package: %s", pkgs[k])
	}
}
