// Copyright 2018-2026 CERN
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

package sciencemesh

import (
	"fmt"
	"time"

	"github.com/cs3org/reva/v3/pkg/ocm/client"
)

// publicOCMTransportConfig builds the runtime transport config for the
// public-only OCM discovery client. It copies the existing integer timeout
// (scaled to a duration) and insecure bool, parses the shared CIDR exception
// list, and leaves AllowLoopback and UseEnvProxy false: ScienceMesh public
// discovery has no loopback topology and stays in direct mode.
//
// The address parser stays in the shared package; this helper only copies
// validated service config into the runtime transport config. It must run
// before any directory fetch so invalid CIDRs abort initialization even when
// the directory list is empty.
func (c *config) publicOCMTransportConfig() (client.TransportConfig, error) {
	cidrs, err := client.ParseFederationCIDRs(c.AllowedFederationCIDRs)
	if err != nil {
		return client.TransportConfig{}, fmt.Errorf("invalid allowed_federation_cidrs: %w", err)
	}
	return client.TransportConfig{
		Timeout:                time.Duration(c.OCMClientTimeout) * time.Second,
		Insecure:               c.OCMClientInsecure,
		AllowedFederationCIDRs: cidrs,
	}, nil
}
