// Copyright 2018-2021 CERN
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

package memory

import (
	"fmt"
	"sync"

	"github.com/cs3org/reva/pkg/registry"
)

// Registry implements the Registry interface.
type Registry struct {
	// m protects async access to the services map.
	sync.Mutex
	// services map a service name with a set of nodes.
	services map[string][]registry.Service
}

// Add implements the Registry interface. If the service is already known in this registry it will only update the nodes.
func (r *Registry) Add(svc registry.Service) error {
	r.Lock()
	defer r.Unlock()

	// init the new service in the registry storage if we have not seen it before
	if _, ok := r.services[svc.Name()]; !ok {
		s := []registry.Service{svc}
		r.services[svc.Name()] = s
		return nil
	}

	// append the requested nodes to the existing service
	for i := range r.services[svc.Name()] {
		ns := make([]node, 0)
		nodes := append(r.services[svc.Name()][i].Nodes(), svc.Nodes()...)
		for n := range nodes {
			ns = append(ns, node{
				id:       nodes[n].ID(),
				address:  nodes[n].Address(),
				metadata: nodes[n].Metadata(),
			})
		}
		r.services[svc.Name()] = []registry.Service{
			service{
				svc.Name(),
				ns,
			},
		}
		fmt.Printf("%+v\n", r.services[svc.Name()])
	}

	return nil
}

// GetService implements the Registry interface. There is currently no load balance being done, but it should not be
// hard to add.
func (r *Registry) GetService(name string) ([]registry.Service, error) {
	r.Lock()
	defer r.Unlock()

	if services, ok := r.services[name]; ok {
		return services, nil
	}

	return nil, fmt.Errorf("service %v not found", name)
}

// New returns an implementation of the Registry interface.
func New(m map[string]interface{}) registry.Registry {
	c, err := registry.ParseConfig(m)
	if err != nil {
		return nil
	}

	r := make(map[string][]registry.Service)
	for sKey, services := range c.Services {
		// allocate as much memory as total services were parsed from config
		r[sKey] = make([]registry.Service, len(services))

		for i := range services {
			s := service{
				name: services[i].Name,
			}

			// copy nodes
			for j := range services[i].Nodes {
				s.nodes = append(s.nodes, node{
					address:  services[i].Nodes[j].Address,
					metadata: services[i].Nodes[j].Metadata,
				})
			}

			r[sKey][i] = &s
		}
	}

	return &Registry{
		services: r,
	}
}
