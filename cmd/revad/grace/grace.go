package grace

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/cernbox/reva/pkg/err"
	"github.com/cernbox/reva/pkg/log"
)

var (
	ctx         = context.Background()
	logger      = log.New("grace")
	errors      = err.New("grace")
	graceful    = os.Getenv("GRACEFUL") == "true"
	parentPID   = os.Getppid()
	listeners   = []net.Listener{}
	srvrs       = []Server{}
	pidFile     string
	childrenPID = []int{}
)

func Exit(errc int) {
	err := removePIDFile()
	if err != nil {
		logger.Error(ctx, err)
	} else {
		logger.Println(ctx, "pidfile got removed")
	}

	os.Exit(errc)
}

func getPIDFromFile(fn string) (int, error) {
	piddata, err := ioutil.ReadFile(fn)
	if err != nil {
		return 0, err
	}
	// Convert the file contents to an integer.
	pid, err := strconv.Atoi(string(piddata))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// Write a pid file, but first make sure it doesn't exist with a running pid.
func WritePIDFile(fn string) error {
	// Read in the pid file as a slice of bytes.
	if piddata, err := ioutil.ReadFile(fn); err == nil {
		// Convert the file contents to an integer.
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			// Look for the pid in the process list.
			if process, err := os.FindProcess(pid); err == nil {
				// Send the process a signal zero kill.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					if !graceful {
						// We only get an error if the pid isn't running, or it's not ours.
						return fmt.Errorf("pid already running: %d", pid)
					}

					if pid != parentPID { // overwrite only if parent pid is pidfile
						// We only get an error if the pid isn't running, or it's not ours.
						return fmt.Errorf("pid %d is not this process parent", pid)
					}
				} else {
					logger.Error(ctx, err)
				}
			} else {
				logger.Error(ctx, err)
			}
		} else {
			logger.Error(ctx, err)
		}
	} else {
		logger.Error(ctx, err)
	}

	// If we get here, then the pidfile didn't exist or we are are in graceful reload and thus we overwrite
	// or the pid in it doesn't belong to the user running this app.
	err := ioutil.WriteFile(fn, []byte(fmt.Sprintf("%d", os.Getpid())), 0664)
	if err != nil {
		return err
	}
	logger.Printf(ctx, "pid file written to %s", fn)
	pidFile = fn
	return nil
}

func newListener(network, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}

// return grpc listener first and http listener second.
func GetListeners(servers []Server) ([]net.Listener, error) {
	srvrs = servers
	lns := []net.Listener{}
	if graceful {
		logger.Println(ctx, "graceful restart, inheriting parent ln fds for grpc and http")
		count := 3
		for _, s := range servers {
			network, addr := s.Network(), s.Address()
			fd := os.NewFile(uintptr(count), "") // 3 because ExtraFile passed to new process
			count++
			ln, err := net.FileListener(fd)
			if err != nil {
				logger.Error(ctx, err)
				// create new fd
				ln, err := newListener(network, addr)
				if err != nil {
					return nil, err
				}
				lns = append(lns, ln)
			} else {
				lns = append(lns, ln)
			}

		}
		// kill parent
		logger.Printf(ctx, "killing parent pid gracefully with SIGQUIT: %d", parentPID)
		syscall.Kill(parentPID, syscall.SIGQUIT)
		listeners = lns
		return lns, nil
	} else {
		// create two listeners for grpc and http
		for _, s := range servers {
			network, addr := s.Network(), s.Address()
			ln, err := newListener(network, addr)
			if err != nil {
				return nil, err
			}
			lns = append(lns, ln)

		}
		listeners = lns
		return lns, nil
	}
}

type Server interface {
	Stop() error
	GracefulStop() error
	Network() string
	Address() string
}

func removePIDFile() error {
	// only remove PID file if the PID written is us
	filePID, err := getPIDFromFile(pidFile)
	if err != nil {
		return err
	}

	if filePID != os.Getpid() {
		return fmt.Errorf("pid in pidfile is different from running pid")
	}

	return os.Remove(pidFile)
}

func TrapSignals() {
	signalCh := make(chan os.Signal, 1024)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
	for {
		select {
		case s := <-signalCh:
			logger.Printf(ctx, "%v signal received", s)
			switch s {
			case syscall.SIGHUP:
				logger.Println(ctx, "preparing for a hot-reload, forking child process...")
				// Fork a child process.
				listeners := getListeners()
				p, err := forkChild(listeners...)
				if err != nil {
					logger.Println(ctx, "unable to fork child process: ", err)
				} else {
					logger.Printf(ctx, "child forked with new pid %d", p.Pid)
					childrenPID = append(childrenPID, p.Pid)
				}

			case syscall.SIGQUIT:
				logger.Println(ctx, "preparing for a graceful shutdown with deadline of 10 seconds")
				go func() {
					count := 10
					for range time.Tick(time.Second) {
						logger.Printf(ctx, "shuting down in %d seconds", count-1)
						count--
						if count <= 0 {
							logger.Println(ctx, "deadline reached before draining active conns, hard stoping ...")
							for _, s := range srvrs {
								s.Stop()
								logger.Printf(ctx, "fd to %s:%s abruptly closed", s.Network(), s.Address())
							}
							Exit(1)
						}
					}
				}()
				for _, s := range srvrs {
					logger.Printf(ctx, "fd to %s:%s gracefully closed ", s.Network(), s.Address())
					s.GracefulStop()
				}
				logger.Println(ctx, "exit with error code 0")
				Exit(0)
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Println(ctx, "preparing for hard shutdown, aborting all conns")
				for _, s := range srvrs {
					logger.Printf(ctx, "fd to %s:%s abruptly closed", s.Network(), s.Address())
					err := s.Stop()
					if err != nil {
						err = errors.Wrap(err, "error stopping server")
						logger.Error(ctx, err)
					}
				}
				Exit(0)
			}
		}
	}
}

func getListenerFile(ln net.Listener) (*os.File, error) {
	switch t := ln.(type) {
	case *net.TCPListener:
		return t.File()
	case *net.UnixListener:
		return t.File()
	}
	return nil, fmt.Errorf("unsupported listener: %T", ln)
}

func forkChild(lns ...net.Listener) (*os.Process, error) {
	// Get the file descriptor for the listener and marshal the metadata to pass
	// to the child in the environment.
	fds := []*os.File{}
	for _, ln := range lns {
		fd, err := getListenerFile(ln)
		if err != nil {
			return nil, err
		}
		fds = append(fds, fd)
	}

	// Pass stdin, stdout, and stderr along with the listener file to the child
	files := []*os.File{
		os.Stdin,
		os.Stdout,
		os.Stderr,
	}
	files = append(files, fds...)

	// Get current environment and add in the listener to it.
	environment := append(os.Environ(), "GRACEFUL=true")

	// Get current process name and directory.
	execName, err := os.Executable()
	if err != nil {
		return nil, err
	}
	execDir := filepath.Dir(execName)

	// Spawn child process.
	p, err := os.StartProcess(execName, os.Args, &os.ProcAttr{
		Dir:   execDir,
		Env:   environment,
		Files: files,
		Sys:   &syscall.SysProcAttr{},
	})

	// TODO(labkode): if the process dies (because config changed and is wrong
	// we need to return an error
	if err != nil {
		return nil, err
	}

	return p, nil
}

func getListeners() []net.Listener {
	return listeners
}
