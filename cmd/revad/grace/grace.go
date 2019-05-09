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

package grace

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Watcher watches a process for a graceful restart
// preserving open network sockets to avoid packets.
type Watcher struct {
	log       zerolog.Logger
	graceful  bool
	ppid      int
	lns       []net.Listener
	ss        []Server
	pidFile   string
	childPIDs []int
}

// Option represent an option.
type Option func(w *Watcher)

// WithLogger adds a logger to the Watcher.
func WithLogger(l zerolog.Logger) Option {
	return func(w *Watcher) {
		w.log = l
	}
}

// WithPIDFile specifies the pid file to use.
func WithPIDFile(fn string) Option {
	return func(w *Watcher) {
		w.pidFile = fn
	}
}

// NewWatcher creates a Watcher.
func NewWatcher(opts ...Option) *Watcher {
	w := &Watcher{
		log:      zerolog.Nop(),
		graceful: os.Getenv("GRACEFUL") == "true",
		ppid:     os.Getppid(),
		pidFile:  path.Join(os.TempDir(), "revad.pid"),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Exit exists the current process cleaning up
// exisiting pid files.
func (w *Watcher) Exit(errc int) {
	err := w.clean()
	if err != nil {
		w.log.Warn().Err(err).Msg("error removing pid file")
	} else {
		w.log.Info().Msgf("pid file %q got removed", w.pidFile)
	}
	os.Exit(errc)
}

func (w *Watcher) clean() error {
	// only remove PID file if the PID has been written by us
	filePID, err := w.readPID()
	if err != nil {
		return err
	}

	if filePID != os.Getpid() {
		// the pidfile may have been changed by a forked child
		// TODO(labkode): is there a way to list children pids for current process?
		return fmt.Errorf("pid:%d in pidfile is different from pid:%d, it can be a leftover from a hard shutdown or that a reload was triggered", filePID, os.Getpid())
	}

	return os.Remove(w.pidFile)
}

func (w *Watcher) readPID() (int, error) {
	piddata, err := ioutil.ReadFile(w.pidFile)
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

// GetProcessFromFile reads the pidfile and returns the running process or error if the process or file
// are not available.
func GetProcessFromFile(pfile string) (*os.Process, error) {
	data, err := ioutil.ReadFile(pfile)
	if err != nil {
		return nil, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return nil, err
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}

	return process, nil
}

// WritePID writes the pid to the configured pid file.
func (w *Watcher) WritePID() error {
	// Read in the pid file as a slice of bytes.
	if piddata, err := ioutil.ReadFile(w.pidFile); err == nil {
		// Convert the file contents to an integer.
		if pid, err := strconv.Atoi(string(piddata)); err == nil {
			// Look for the pid in the process list.
			if process, err := os.FindProcess(pid); err == nil {
				// Send the process a signal zero kill.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					if !w.graceful {
						return fmt.Errorf("pid already running: %d", pid)
					}

					if pid != w.ppid { // overwrite only if parent pid is pidfile
						// We only get an error if the pid isn't running, or it's not ours.
						return fmt.Errorf("pid %d is not this process parent", pid)
					}
				} else {
					w.log.Warn().Err(err).Msg("error sending zero kill signal to current process")
				}
			} else {
				w.log.Warn().Msgf("pid:%d not found", pid)
			}
		} else {
			w.log.Warn().Msg("error casting contents of pidfile to pid(int)")
		}
	} else {
		w.log.Warn().Msg("error reading pidfile")
	}

	// If we get here, then the pidfile didn't exist or we are are in graceful reload and thus we overwrite
	// or the pid in it doesn't belong to the user running this app.
	err := ioutil.WriteFile(w.pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0664)
	if err != nil {
		return err
	}
	w.log.Info().Msgf("pidfile written to %s", w.pidFile)
	return nil
}

func newListener(network, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}

// GetListeners return grpc listener first and http listener second.
func (w *Watcher) GetListeners(servers []Server) ([]net.Listener, error) {
	w.ss = servers
	lns := []net.Listener{}
	if w.graceful {
		w.log.Info().Msg("graceful restart, inheriting parent ln fds for grpc and http")
		count := 3
		for _, s := range servers {
			network, addr := s.Network(), s.Address()
			fd := os.NewFile(uintptr(count), "") // 3 because ExtraFile passed to new process
			count++
			ln, err := net.FileListener(fd)
			if err != nil {
				w.log.Error().Err(err).Msg("error creating net.Listener from fd")
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
		// TODO(labkode): maybe race condition here?
		// What do we do if we cannot kill the parent but we have valid fds?
		// Do we abort running the forked child? Probably yes, as if the parent cannot be
		// killed that means we run two version of the code indefinetely.
		w.log.Info().Msgf("killing parent pid gracefully with SIGQUIT: %d", w.ppid)
		err := syscall.Kill(w.ppid, syscall.SIGQUIT)
		if err != nil {
			w.log.Error().Err(err).Msgf("error killing parent process with ppid:%d", w.ppid)
			err = errors.Wrap(err, "error killing parent process")
			return nil, err
		}
		w.lns = lns
		return lns, nil
	}

	// create two listeners for grpc and http
	for _, s := range servers {
		network, addr := s.Network(), s.Address()
		ln, err := newListener(network, addr)
		if err != nil {
			return nil, err
		}
		lns = append(lns, ln)

	}
	w.lns = lns
	return lns, nil
}

// Server is the interface that servers like HTTP or gRPC
// servers need to implement.
type Server interface {
	Stop() error
	GracefulStop() error
	Network() string
	Address() string
}

// TrapSignals captures the OS signal.
func (w *Watcher) TrapSignals() {
	signalCh := make(chan os.Signal, 1024)
	signal.Notify(signalCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT)
	for {
		s := <-signalCh
		w.log.Info().Msgf("%v signal received", s)

		switch s {
		case syscall.SIGHUP:
			w.log.Info().Msg("preparing for a hot-reload, forking child process...")

			// Fork a child process.
			listeners := w.lns
			p, err := forkChild(listeners...)
			if err != nil {
				w.log.Error().Err(err).Msgf("unable to fork child process")
			} else {
				w.log.Info().Msgf("child forked with new pid %d", p.Pid)
				w.childPIDs = append(w.childPIDs, p.Pid)
			}

		case syscall.SIGQUIT:
			w.log.Info().Msg("preparing for a graceful shutdown with deadline of 10 seconds")
			go func() {
				count := 10
				for range time.Tick(time.Second) {
					w.log.Info().Msgf("shuting down in %d seconds", count-1)
					count--
					if count <= 0 {
						w.log.Info().Msg("deadline reached before draining active conns, hard stoping ...")
						for _, s := range w.ss {
							err := s.Stop()
							if err != nil {
								w.log.Error().Err(err).Msg("error stoping server")
							}
							w.log.Info().Msgf("fd to %s:%s abruptly closed", s.Network(), s.Address())
						}
						w.Exit(1)
					}
				}
			}()
			for _, s := range w.ss {
				w.log.Info().Msgf("fd to %s:%s gracefully closed ", s.Network(), s.Address())
				err := s.GracefulStop()
				if err != nil {
					w.log.Error().Err(err).Msg("error stoping server")
					w.log.Info().Msg("exit with error code 1")
					w.Exit(1)
				}
			}
			w.log.Info().Msg("exit with error code 0")
			w.Exit(0)
		case syscall.SIGINT, syscall.SIGTERM:
			w.log.Info().Msg("preparing for hard shutdown, aborting all conns")
			for _, s := range w.ss {
				w.log.Info().Msgf("fd to %s:%s abruptly closed", s.Network(), s.Address())
				err := s.Stop()
				if err != nil {
					w.log.Error().Err(err).Msg("error stoping server")
				}
			}
			w.Exit(0)
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
	fmt.Println(execName, os.Args)
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
