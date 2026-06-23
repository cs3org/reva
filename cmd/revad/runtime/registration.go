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
	"fmt"
	"maps"
	"net"
	"os"
	"time"

	"github.com/cs3org/reva/v3/cmd/revad/pkg/config"
	"github.com/cs3org/reva/v3/pkg/registry"
	"github.com/cs3org/reva/v3/pkg/rhttp/global"
	"github.com/cs3org/reva/v3/pkg/service"
	"github.com/rs/zerolog"
)

// buildRegistry builds this Reva instance's registry from config (one per Reva,
// never global). A WithRegistry override takes precedence.
func buildRegistry(cfg *config.Config, override registry.Registry, log *zerolog.Logger) (registry.Registry, error) {
	if override != nil {
		return override, nil
	}
	rc := cfg.Shared.Registry
	driverCfg := map[string]any{}
	if rc.Drivers != nil {
		if d, ok := rc.Drivers[rc.Driver]; ok && d != nil {
			driverCfg = d
		}
	}
	// Liveness thresholds are applied by the shared BaseRegistry.
	thresholds := registry.Thresholds{
		DegradedAfter: parseDurationOr(rc.DegradedAfter, 0),
		OfflineAfter:  parseDurationOr(rc.OfflineAfter, 0),
		ReapAfter:     parseDurationOr(rc.ReapAfter, 0),
	}
	reg, err := registry.New(rc.Driver, driverCfg, thresholds)
	if err != nil {
		return nil, fmt.Errorf("runtime: building service registry: %w", err)
	}
	log.Info().Str("driver", driverOrDefault(rc.Driver)).Msg("service registry initialized")
	return reg, nil
}

func driverOrDefault(d string) string {
	if d == "" {
		return "memory"
	}
	return d
}

// register records one node per loaded service after the listeners have bound.
func (r *Reva) register() {
	r.addNodes("registered service")
}

// heartbeat re-adds this process's nodes to refresh their liveness.
func (r *Reva) heartbeat() {
	r.addNodes("service heartbeat")
}

// addNodes adds one node per loaded service, logging each with the given msg.
func (r *Reva) addNodes(msg string) {
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	for _, srv := range r.servers {
		// Advertise a reachable "host:port": the listener's bind host (e.g.
		// "[::]" or "0.0.0.0") is not routable, so replace it with the hostname.
		addr := hostPort(hostname, srv.listener.Addr().String())
		for name, impl := range srv.services {
			node := registry.NewNode(
				nodeID(addr, pid, name),
				addr,
				nodeMetadata(srv, hostname, pid, impl),
			)
			if err := r.registry.Add(registry.NewService(name, []registry.Node{node})); err != nil {
				r.log.Error().Err(err).Str("service", name).Msg("failed to register service")
				continue
			}
			r.log.Trace().Str("service", name).Str("address", addr).Msg(msg)
		}
	}
}

// hostPort returns "host:port" from a listener address. A wildcard bind host
// ("::", "0.0.0.0" or empty) is not routable, so it is replaced with the
// hostname; a concrete bind host is already reachable and kept as-is.
func hostPort(hostname, addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if isWildcard(host) {
		return net.JoinHostPort(hostname, port)
	}
	return net.JoinHostPort(host, port)
}

// isWildcard reports whether host is an unspecified bind address.
func isWildcard(host string) bool {
	if host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsUnspecified()
	}
	return false
}

// nodeMetadata builds a node's metadata: framework-derived keys plus, for HTTP
// services, scheme/prefix, plus any service-owned keys from MetadataProvider.
func nodeMetadata(srv *Server, hostname string, pid int, impl any) map[string]string {
	meta := map[string]string{
		"transport":           srv.transport,
		"host":                hostname,
		"pid":                 fmt.Sprintf("%d", pid),
		registry.MetaState:    registry.StateReady,
		registry.MetaLastSeen: time.Now().UTC().Format(time.RFC3339),
	}
	if srv.transport == "http" {
		meta[registry.MetaScheme] = srv.scheme
		if hs, ok := impl.(global.Service); ok {
			meta[registry.MetaPrefix] = hs.Prefix()
		}
	}
	if mp, ok := impl.(service.MetadataProvider); ok {
		maps.Copy(meta, mp.RegistryMetadata())
	}
	return meta
}

// nodeID is a stable per-process identity for a service node, of the form
// "<host:port>#<pid>/<service>".
func nodeID(addr string, pid int, service string) string {
	return fmt.Sprintf("%s#%d/%s", addr, pid, service)
}

// startHeartbeat re-registers this process's nodes on an interval to keep them
// live.
func (r *Reva) startHeartbeat() {
	if r.heartbeatInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(r.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				r.heartbeat()
			}
		}
	}()
}

// deregister removes this process's nodes on graceful shutdown.
func (r *Reva) deregister() {
	if r.registry == nil {
		return
	}
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	for _, srv := range r.servers {
		addr := hostPort(hostname, srv.listener.Addr().String())
		for name := range srv.services {
			node := registry.NewNode(
				nodeID(addr, pid, name),
				addr,
				map[string]string{registry.MetaState: registry.StateDraining},
			)
			if err := r.registry.Remove(registry.NewService(name, []registry.Node{node})); err != nil {
				r.log.Error().Err(err).Str("service", name).Msg("failed to deregister service")
			}
		}
	}
}
