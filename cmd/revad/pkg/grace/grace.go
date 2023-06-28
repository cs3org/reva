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

package grace

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

// Watcher watches a process for a graceful restart
// preserving open network sockets to avoid packet loss.
type Watcher struct {
	log       zerolog.Logger
	graceful  bool
	ppid      int
	lns       map[string]net.Listener
	ss        []Server
	SL        Serverless
	pidFile   string
	childPIDs []int
}

const revaEnvPrefix = "REVA_FD_"

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
		ss:       make([]Server, 0),
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Exit exits the current process cleaning up
// existing pid files.
func (w *Watcher) Exit(errc int) {
	w.Clean()
	os.Exit(errc)
}

// Clean cleans up existing pid files.
func (w *Watcher) Clean() {
	err := w.clean()
	if err != nil {
		w.log.Warn().Err(err).Msg("error removing pid file")
	} else {
		w.log.Info().Msgf("pid file %q got removed", w.pidFile)
	}
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
	piddata, err := os.ReadFile(w.pidFile)
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
	data, err := os.ReadFile(pfile)
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
	if piddata, err := os.ReadFile(w.pidFile); err == nil {
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
	} // else {
	// w.log.Info().Msg("error reading pidfile")
	//}

	// If we get here, then the pidfile didn't exist or we are in graceful reload and thus we overwrite
	// or the pid in it doesn't belong to the user running this app.
	err := os.WriteFile(w.pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0664)
	if err != nil {
		return err
	}
	w.log.Info().Msgf("pidfile saved at: %s", w.pidFile)
	return nil
}

func newListener(network, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}

// implements the net.Listener interface.
type inherited struct {
	f  *os.File
	ln net.Listener
}

func (i *inherited) Accept() (net.Conn, error) {
	return i.ln.Accept()
}

func (i *inherited) Close() error {
	// TODO: improve this: if file close has error
	// the listener is not closed
	if err := i.f.Close(); err != nil {
		return err
	}
	return i.ln.Close()
}

func (i *inherited) Addr() net.Addr {
	return i.ln.Addr()
}

func inheritedListeners() map[string]net.Listener {
	lns := make(map[string]net.Listener)
	for _, val := range os.Environ() {
		if strings.HasPrefix(val, revaEnvPrefix) {
			// env variable is of type REVA_FD_<svcname>=<fd>
			env := strings.TrimPrefix(val, revaEnvPrefix)
			s := strings.Split(env, "=")
			if len(s) != 2 {
				continue
			}
			svcname := s[0]
			fd, err := strconv.ParseUint(s[1], 10, 64)
			if err != nil {
				continue
			}
			f := os.NewFile(uintptr(fd), "")
			ln, err := net.FileListener(f)
			if err != nil {
				// TODO: log error
				continue
			}
			lns[svcname] = &inherited{f: f, ln: ln}
		}
	}
	return lns
}

func isRandomAddress(addr string) bool {
	return addr == ""
}

func getAddress(addr string) string {
	if isRandomAddress(addr) {
		return ":0"
	}
	return addr
}

// GetListeners return grpc listener first and http listener second.
func (w *Watcher) GetListeners(servers map[string]Addressable) (map[string]net.Listener, error) {
	lns := make(map[string]net.Listener)

	if w.graceful {
		w.log.Info().Msg("graceful restart, inheriting parent listener fds for grpc and http services")

		inherited := inheritedListeners()
		for svc, ln := range inherited {
			addr, ok := servers[svc]
			if !ok {
				continue
			}
			// for services with random addresses, check and assign if available from inherited
			// from the assigned addresses, assing the listener if address correspond
			if isRandomAddress(addr.Address()) || addr.Address() == ln.Addr().String() { // TODO: check which is the host here
				lns[svc] = ln
			}
		}

		// close all the listeners not used from inherited
		for svc, ln := range inherited {
			if _, ok := lns[svc]; !ok {
				if err := ln.Close(); err != nil {
					w.log.Error().Err(err).Msgf("error closing inherited listener %s", ln.Addr().String())
					return nil, errors.Wrap(err, "error closing inherited listener")
				}
			}
		}

		// create assigned/random listeners for the missing services
		for svc, addr := range servers {
			_, ok := lns[svc]
			if ok {
				continue
			}
			a := getAddress(addr.Address())
			ln, err := newListener(addr.Network(), a)
			if err != nil {
				w.log.Error().Err(err).Msgf("error getting listener on %s", a)
				return nil, errors.Wrap(err, "error getting listener")
			}
			lns[svc] = ln
		}

		// kill parent
		// TODO(labkode): maybe race condition here?
		// What do we do if we cannot kill the parent but we have valid fds?
		// Do we abort running the forked child? Probably yes, as if the parent cannot be
		// killed that means we run two version of the code indefinitely.
		w.log.Info().Msgf("killing parent pid gracefully with SIGQUIT: %d", w.ppid)
		p, err := os.FindProcess(w.ppid)
		if err != nil {
			w.log.Error().Err(err).Msgf("error finding parent process with ppid:%d", w.ppid)
			err = errors.Wrap(err, "error finding parent process")
			return nil, err
		}
		err = p.Kill()
		if err != nil {
			w.log.Error().Err(err).Msgf("error killing parent process with ppid:%d", w.ppid)
			err = errors.Wrap(err, "error killing parent process")
			return nil, err
		}
		w.lns = lns
		return lns, nil
	}

	// no graceful
	for svc, s := range servers {
		network, addr := s.Network(), getAddress(s.Address())
		// multiple services may have the same listener
		ln, ok := get(lns, addr, network)
		if ok {
			lns[svc] = ln
			continue
		}
		ln, err := newListener(network, addr)
		if err != nil {
			return nil, err
		}
		lns[svc] = ln
	}
	w.lns = lns
	return lns, nil
}

func get(lns map[string]net.Listener, address, network string) (net.Listener, bool) {
	for _, ln := range lns {
		if addressEqual(ln.Addr(), network, address) {
			return ln, true
		}
	}
	return nil, false
}

func addressEqual(a net.Addr, network, address string) bool {
	if a.Network() != network {
		return false
	}

	switch network {
	case "tcp":
		t, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			return false
		}
		return tcpAddressEqual(a.(*net.TCPAddr), t)
	case "unix":
		t, err := net.ResolveUnixAddr(network, address)
		if err != nil {
			return false
		}
		return unixAddressEqual(a.(*net.UnixAddr), t)
	}
	return false
}

func tcpAddressEqual(a1, a2 *net.TCPAddr) bool {
	return a1.Port == a2.Port
}

func unixAddressEqual(a1, a2 *net.UnixAddr) bool {
	return a1.Name == a2.Name && a1.Net == a2.Net
}

type Addressable interface {
	Network() string
	Address() string
}

// Server is the interface that servers like HTTP or gRPC
// servers need to implement.
type Server interface {
	Start(net.Listener) error
	Serverless
	Addressable
}

// Serverless is the interface that the serverless server implements.
type Serverless interface {
	Stop() error
	GracefulStop() error
}

func (w *Watcher) SetServers(s []Server)      { w.ss = s }
func (w *Watcher) SetServerless(s Serverless) { w.SL = s }

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
			p, err := forkChild(listeners)
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
				ticker := time.NewTicker(time.Second)
				for ; true; <-ticker.C {
					w.log.Info().Msgf("shutting down in %d seconds", count-1)
					count--
					if count <= 0 {
						w.log.Info().Msg("deadline reached before draining active conns, hard stopping ...")
						for _, s := range w.ss {
							err := s.Stop()
							if err != nil {
								w.log.Error().Err(err).Msg("error stopping server")
							}
							w.log.Info().Msgf("fd to %s:%s abruptly closed", s.Network(), s.Address())
						}
						err := w.SL.Stop()
						if err != nil {
							w.log.Error().Err(err).Msg("error stopping serverless server")
						}
						w.log.Info().Msg("serverless services abruptly closed")
						w.Exit(1)
					}
				}
			}()
			for _, s := range w.ss {
				w.log.Info().Msgf("fd to %s:%s gracefully closed ", s.Network(), s.Address())
				err := s.GracefulStop()
				if err != nil {
					w.log.Error().Err(err).Msg("error stopping server")
					w.log.Info().Msg("exit with error code 1")
					w.Exit(1)
				}
			}
			if w.SL != nil {
				err := w.SL.GracefulStop()
				if err != nil {
					w.log.Error().Err(err).Msg("error stopping server")
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
					w.log.Error().Err(err).Msg("error stopping server")
				}
			}
			err := w.SL.Stop()
			if err != nil {
				w.log.Error().Err(err).Msg("error stopping serverless server")
			}
			w.log.Info().Msg("serverless services abruptly closed")

			w.Exit(0)
		}
	}
}

func getListenerFile(ln net.Listener) (*os.File, error) {
	switch t := ln.(type) {
	case *inherited:
		return t.f, nil
	case *net.TCPListener:
		return t.File()
	case *net.UnixListener:
		return t.File()
	}
	return nil, fmt.Errorf("unsupported listener: %T", ln)
}

func forkChild(lns map[string]net.Listener) (*os.Process, error) {
	// Get the file descriptor for the listener and marshal the metadata to pass
	// to the child in the environment.
	fds := make(map[string]*os.File, 0)
	for name, ln := range lns {
		fd, err := getListenerFile(ln)
		if err != nil {
			return nil, err
		}
		fds[name] = fd
	}

	// Pass stdin, stdout, and stderr along with the listener file to the child
	files := []*os.File{
		os.Stdin,
		os.Stdout,
		os.Stderr,
	}

	// Get current environment and add in the listener to it.
	environment := append(os.Environ(), "GRACEFUL=true")
	counter := 3
	for k, fd := range fds {
		k = strings.ToUpper(k)
		environment = append(environment, fmt.Sprintf("%s%s=%d", revaEnvPrefix, k, counter))
		files = append(files, fd)
		counter++
	}

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

	// TODO(labkode): if the process dies (because config changed and is wrong)
	// we need to return an error
	if err != nil {
		return nil, err
	}

	return p, nil
}
