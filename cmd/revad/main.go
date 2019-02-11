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

	_ "github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/credential/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/token/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/auth/tokenwriter/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/handlers/loader"
	_ "github.com/cernbox/reva/cmd/revad/svcs/httpsvcs/loader"
	_ "github.com/cernbox/reva/pkg/auth/manager/loader"
	_ "github.com/cernbox/reva/pkg/storage/broker/loader"
	_ "github.com/cernbox/reva/pkg/storage/fs/loader"
	_ "github.com/cernbox/reva/pkg/token/manager/loader"
	_ "github.com/cernbox/reva/pkg/user/manager/loader"

	"github.com/cernbox/reva/cmd/revad/config"
	"github.com/cernbox/reva/cmd/revad/grace"
	"github.com/cernbox/reva/cmd/revad/grpcserver"
	"github.com/cernbox/reva/cmd/revad/httpserver"

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

	servers := []grace.Server{}
	if !conf.DisableHTTP {
		servers = append(servers, getHTTPServer())
	}

	if !conf.DisableGRPC {
		servers = append(servers, getGRPCServer())
	}

	listeners, err := grace.GetListeners(servers)
	if err != nil {
		logger.Error(ctx, err)
		grace.Exit(1)
	}

	if !conf.DisableHTTP {
		go func() {
			if err := servers[0].(*httpserver.Server).Start(listeners[0]); err != nil {
				err = errors.Wrap(err, "error starting grpc server")
				logger.Error(ctx, err)
				grace.Exit(1)
			}
		}()
	}

	if !conf.DisableGRPC {
		go func() {
			if err := servers[1].(*grpcserver.Server).Start(listeners[1]); err != nil {
				err = errors.Wrap(err, "error starting http server")
				logger.Error(ctx, err)
				grace.Exit(1)
			}
		}()
	}

	grace.TrapSignals()
}

func getGRPCServer() *grpcserver.Server {
	s, err := grpcserver.New(config.Get("grpc"))
	if err != nil {
		logger.Error(ctx, err)
		grace.Exit(1)
	}
	return s
}

func getHTTPServer() *httpserver.Server {
	s, err := httpserver.New(config.Get("http"))
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

	// apply defaults
}

//  tweakCPU parses string cpu and sets GOMAXPROCS
// according to its value. It accepts either
// a number (e.g. 3) or a percent (e.g. 50%).
func tweakCPU() error {
	cpu := conf.MaxCPUs
	var numCPU int

	availCPU := runtime.NumCPU()

	if cpu != "" {
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
	}

	if numCPU > availCPU || numCPU == 0 {
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
	MaxCPUs     string `mapstructure:"max_cpus"`
	LogFile     string `mapstructure:"log_file"`
	LogMode     string `mapstructure:"log_mode"`
	DisableHTTP bool   `mapstructure:"disable_http"`
	DisableGRPC bool   `mapstructure:"disable_grpc"`
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
