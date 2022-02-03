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

package runtime

import (
	"context"
	"time"

	"github.com/cs3org/reva/pkg/registry"
	"github.com/cs3org/reva/pkg/utils"
	"github.com/rs/zerolog"
)

type Service struct {
	name  string
	nodes []Node
}

func (s Service) Name() string {
	return s.name
}

func (s Service) Nodes() []registry.Node {
	nodes := []registry.Node{}
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

type Node struct {
	address  string
	metadata map[string]string
	id       string
}

func (n Node) Address() string {
	return n.address
}

func (n Node) Metadata() map[string]string {
	return n.metadata
}

func (n Node) ID() string {
	return n.id
}

func registerEndpoint(ctx context.Context, serviceName, serviceID, addr, protocol string, logger *zerolog.Logger, registrationRefresh time.Duration) error {
	node := Node{
		id:       serviceName + "-" + serviceID,
		address:  addr,
		metadata: make(map[string]string),
	}

	node.metadata["server"] = protocol
	node.metadata["transport"] = protocol
	node.metadata["protocol"] = protocol

	service := &Service{
		name:  serviceName,
		nodes: []Node{node},
	}

	logger.Info().Msgf("registering service %v@%v", serviceName, node.Address)
	if err := utils.GlobalRegistry.Add(service); err != nil {
		logger.Fatal().Err(err).Msgf("Registration error for service %v", serviceName)
		return err
	}

	// refresh registration if non zero refresh interval
	if registrationRefresh > 0*time.Second {
		t := time.NewTicker(registrationRefresh)

		go func() {
			for {
				select {
				case <-t.C:
					logger.Debug().Interface("service", service).Msg("refreshing registration")
					if err := utils.GlobalRegistry.Add(service); err != nil {
						logger.Error().Err(err).Msgf("registration error for service %v", serviceID)
					}
				case <-ctx.Done():
					t.Stop()
					return
				}
			}
		}()
	}

	return nil
}
