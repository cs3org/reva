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

package registry

// Registry is the source of truth for where services live: a process registers
// its services (Add) and resolves peers by name (GetService).
type Registry interface {
	Add(Service) error
	GetService(string) (Service, error)
	ListServices() ([]Service, error)
	Remove(Service) error
	// Watch streams add/remove events; the memory backend never delivers.
	Watch() (<-chan Event, error)
}

// Service is a named set of nodes.
type Service interface {
	Name() string
	Nodes() []Node
}

// Node is a single instance serving a service.
type Node interface {
	Address() string
	ID() string
	// Metadata carries transport, version, host, pid, state and last_seen, and
	// distinguishes implementations (e.g. auth basic vs bearer).
	Metadata() map[string]string
}

type EventType string

const (
	EventAdd    EventType = "add"
	EventRemove EventType = "remove"
)

// Event is a change to the registry; for EventRemove the Node carries at least
// the ID.
type Event struct {
	Type    EventType
	Service string
	Node    Node
}

// Metadata keys and node states.
const (
	MetaState     = "state"
	MetaLastSeen  = "last_seen"
	MetaScheme    = "scheme"     // "http" | "https" for HTTP services
	MetaPrefix    = "prefix"     // HTTP URL path prefix
	MetaPublicURL = "public_url" // explicit external URL override
	MetaMountID   = "mount_id"   // storage affinity key (data provider)

	StateReady    = "ready"
	StateDegraded = "degraded"
	StateOffline  = "offline"
	StateDraining = "draining"
)
